package run

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"github.com/distribution/reference"
	"github.com/docker/cli/cli/config"
	clitypes "github.com/docker/cli/cli/config/types"
	"github.com/docker/docker/api/types/container"
	dockercontainer "github.com/docker/docker/api/types/container"
	imagetypes "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/registry"
	"github.com/go-logr/logr"
	"github.com/moby/buildkit/client"
	gwclient "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth/authprovider"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	"github.com/raffis/rageta/internal/buildkit/vertex"
	"github.com/raffis/rageta/internal/setup/buildkitsetup"
	"github.com/raffis/rageta/internal/setup/flagset"
	"github.com/spf13/pflag"
	"github.com/tonistiigi/fsutil"
)

const (
	buildkitdContainerName = "rageta-buildkitd"
	buildkitdImage         = "rageta-buildkit:latest"
)

type BuildkitOptions struct {
	BuildkitOptions buildkitsetup.Options
	CacheImports    []string
	CacheExports    []string
	BuildContext    string
	NoCache         bool
}

func (s BuildkitOptions) Build() Task {
	return &Buildkit{
		opts: s,
	}
}

func NewBuildkitOptions() BuildkitOptions {
	return BuildkitOptions{
		BuildkitOptions: buildkitsetup.NewOptions(),
		BuildContext:    ".",
	}
}

func (s *BuildkitOptions) BindFlags(flags flagset.Interface) {
	buildkitFlags := pflag.NewFlagSet("Buildkit", pflag.ExitOnError)
	buildkitFlags.StringArrayVarP(&s.CacheImports, "cache-from", "", s.CacheImports, "Import build cache, e.g. type=registry,ref=example.com/foo/bar, or type=local,src=path/to/dir")
	buildkitFlags.StringArrayVarP(&s.CacheExports, "cache-to", "", s.CacheExports, "Export build cache, e.g. type=registry,ref=example.com/foo/bar, or type=local,dest=path/to/dir")
	buildkitFlags.BoolVar(&s.NoCache, "no-cache", s.NoCache, "Disable cache for all the vertices")
	buildkitFlags.StringVar(&s.BuildContext, "build-context", s.BuildContext, "Path to build context directory (source root for local: sources)")
	s.BuildkitOptions.BindFlags(buildkitFlags)
	flags.AddFlagSet(buildkitFlags)
}

type Buildkit struct {
	opts BuildkitOptions
}

type BuildkitContext struct {
	Client         *client.Client
	GatewayClient  gwclient.Client
	VertexRouter   *vertex.Router
	CacheImports   []client.CacheOptionsEntry
	GWCacheImports []gwclient.CacheOptionsEntry
	CacheExports   []client.CacheOptionsEntry
	NoCache        bool
	ContextFS      fsutil.FS
	BuiltRefs      []gwclient.Reference
}

func (s *Buildkit) Label() string {
	return "Connecting to buildkit"
}

func (s *Buildkit) Run(rc *RunContext, next Next) error {
	if s.opts.BuildkitOptions.Host == "docker-container://rageta-buildkitd" {
		if err := s.ensureBuildkitd(rc); err != nil {
			return err
		}
	}

	c, err := s.opts.BuildkitOptions.Build(rc)
	if err != nil {
		return err
	}

	cacheImports, err := buildkitsetup.ParseImportCache(s.opts.CacheImports)
	if err != nil {
		return err
	}

	cacheExports, err := buildkitsetup.ParseExportCache(s.opts.CacheExports)
	if err != nil {
		return err
	}

	gwCacheImports := make([]gwclient.CacheOptionsEntry, len(cacheImports))
	for i, e := range cacheImports {
		gwCacheImports[i] = gwclient.CacheOptionsEntry{Type: e.Type, Attrs: e.Attrs}
	}

	vertexRouter := vertex.NewRouter()
	contextFS, err := fsutil.NewFS(s.opts.BuildContext)
	if err != nil {
		return err
	}

	rc.Buildkit.Client = c
	rc.Buildkit.CacheImports = cacheImports
	rc.Buildkit.GWCacheImports = gwCacheImports
	rc.Buildkit.CacheExports = cacheExports
	rc.Buildkit.NoCache = s.opts.NoCache
	rc.Buildkit.VertexRouter = vertexRouter
	rc.Buildkit.ContextFS = contextFS

	buildOpt := client.SolveOpt{
		FrontendAttrs: map[string]string{},
		LocalMounts: map[string]fsutil.FS{
			"context": contextFS,
		},
		CacheImports: cacheImports,
		CacheExports: cacheExports,
		Session: []session.Attachable{
			authprovider.NewDockerAuthProvider(authprovider.DockerAuthProviderConfig{
				AuthConfigProvider: authprovider.LoadAuthConfig(config.LoadDefaultConfigFile(io.Discard)),
			}),
			secretsprovider.NewSecretProvider(rc.Secrets.Store),
		},
	}

	ch := make(chan *client.SolveStatus)

	go func() {
		for status := range ch {
			vertexRouter.Route(status)
		}
	}()

	_, err = c.Build(rc, buildOpt, "", func(ctx context.Context, gwc gwclient.Client) (*gwclient.Result, error) {
		rc.Buildkit.GatewayClient = gwc
		err := next(rc)
		res := gwclient.NewResult()
		if len(rc.Buildkit.BuiltRefs) > 0 {
			res.SetRef(rc.Buildkit.BuiltRefs[len(rc.Buildkit.BuiltRefs)-1])
		}
		return res, err
	}, ch)

	return err
}

func (s *Buildkit) ensureBuildkitd(rc *RunContext) error {
	ctx := rc.Context
	logger := rc.Logging.Logger

	var cli *dockerclient.Client
	var err error
	cli, err = dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create docker client: %w", err)
	}

	defer cli.Close()

	inspect, err := cli.ContainerInspect(ctx, buildkitdContainerName)
	if err == nil {
		if inspect.State.Running {
			return nil
		}

		if cli.ContainerStart(ctx, inspect.ID, dockercontainer.StartOptions{}) != nil {
			return fmt.Errorf("failed to start buildkitd container: %w", err)
		}
	}

	if !dockerclient.IsErrNotFound(err) {
		return fmt.Errorf("failed to inspect buildkitd container: %w", err)
	}

	if err := ensureImage(rc, cli, logger, buildkitdImage); err != nil {
		return err
	}

	vol, err := cli.VolumeInspect(ctx, "rageta-containerd")

	if err != nil {
		vol, err = cli.VolumeCreate(rc, volume.CreateOptions{
			Name: "rageta-containerd",
		})

		if err != nil {
			return err
		}
	}

	containerConfig := &dockercontainer.Config{
		Image: buildkitdImage,
		Env: []string{
			"BUILDKIT_STEP_LOG_MAX_SIZE=-1",
		},
	}

	if rc.Otel.Endpoint != "" {
		containerConfig.Env = append(containerConfig.Env, "OTEL_TRACES_EXPORTER=otlp")
		containerConfig.Env = append(containerConfig.Env, fmt.Sprintf("OTEL_EXPORTER_OTLP_ENDPOINT=%s", rc.Otel.Endpoint))
	}

	info, err := cli.Info(ctx)
	if err != nil {
		return err
	}

	totalMem := info.MemTotal
	numCPU := info.NCPU

	memLimit := int64(float64(totalMem) * 0.75)
	nanoCPUs := int64(float64(numCPU) * 0.75 * 1e9)

	resources := container.Resources{
		Memory:     memLimit,
		MemorySwap: memLimit,
		NanoCPUs:   nanoCPUs,
	}

	hostConfig := &dockercontainer.HostConfig{
		Privileged: true,
		RestartPolicy: dockercontainer.RestartPolicy{
			Name: dockercontainer.RestartPolicyAlways,
		},
		Resources: resources,
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeVolume,
				Source: vol.Name,
				Target: "/var/lib/containerd",
			},
		},
	}

	logger.V(1).Info("creating buildkitd container", "container", buildkitdContainerName, "image", buildkitdImage)
	created, err := cli.ContainerCreate(ctx, containerConfig, hostConfig, &network.NetworkingConfig{}, nil, buildkitdContainerName)
	if err != nil {
		return fmt.Errorf("failed to create buildkitd container: %w", err)
	}

	return cli.ContainerStart(ctx, created.ID, dockercontainer.StartOptions{})
}

type dockerAuth interface {
	GetAuthConfig(registryHostname string) (clitypes.AuthConfig, error)
}

func encodedAuth(ref reference.Named, configFile dockerAuth) (string, error) {
	repoInfo, err := registry.ParseRepositoryInfo(ref)
	if err != nil {
		return "", err
	}

	key := registry.GetAuthConfigKey(repoInfo.Index)
	authConfig, err := configFile.GetAuthConfig(key)
	if err != nil {
		return "", err
	}

	buf, err := json.Marshal(authConfig)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(buf), nil
}

// ensureImage pulls image if it isn't already present locally.
func ensureImage(rc *RunContext, cli *dockerclient.Client, logger logr.Logger, image string) error {
	images, err := cli.ImageList(rc, imagetypes.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list images: %w", err)
	}

	for _, img := range images {
		if slices.Contains(img.RepoTags, image) {
			return nil
		}
	}

	logger.V(1).Info("pulling image", "image", image)

	ref, err := reference.ParseNormalizedNamed(image)
	if err != nil {
		return err
	}

	auth, err := encodedAuth(ref, config.LoadDefaultConfigFile(io.Discard))
	if err != nil {
		return err
	}

	r, err := cli.ImagePull(rc, image, imagetypes.PullOptions{RegistryAuth: auth})
	if err != nil {
		return fmt.Errorf("failed to pull image `%s`: %w", image, err)
	}
	defer r.Close()

	if err := jsonmessage.DisplayJSONMessagesStream(r, rc.Display.Stdout, 0, false, nil); err != nil {
		return fmt.Errorf("failed to pull image `%s`: %w", image, err)
	}

	return nil
}
