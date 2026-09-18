package run

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

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
	"github.com/raffis/rageta/internal/setup/buildkitsetup"
	"github.com/raffis/rageta/internal/setup/flagset"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	buildkitdContainerName = "rageta-buildkitd"
	buildkitdImage         = "rageta-buildkitd:latest"
)

type BuildkitClaimOptions struct {
	Buildkit    *buildkitsetup.Options
	MemoryLimit string
	CPULimit    string
}

func (s BuildkitClaimOptions) Build() Task {
	return &BuildkitClaim{
		opts: s,
	}
}

func NewBuildkitClaimOptions() BuildkitClaimOptions {
	return BuildkitClaimOptions{}
}

func (s *BuildkitClaimOptions) BindFlags(flags flagset.Interface) {
	buildkitFlags := pflag.NewFlagSet("Buildkit Provisioning", pflag.ExitOnError)
	buildkitFlags.StringVarP(&s.MemoryLimit, "buildkit-memorylimit", "", s.MemoryLimit, "Memory limit")
	buildkitFlags.StringVarP(&s.CPULimit, "buildkit-cpulimit", "", s.CPULimit, "CPU limit")
	flags.AddFlagSet(buildkitFlags)
}

type BuildkitClaim struct {
	opts BuildkitClaimOptions
}

func (s *BuildkitClaim) Label() string {
	return "Provisioning buildkit"
}

func (s *BuildkitClaim) Run(rc *RunContext, next Next) error {
	if s.opts.Buildkit.Host == "docker-container://rageta-buildkitd" {
		if err := s.ensureBuildkitd(rc); err != nil {
			return err
		}
	}

	return next(rc)
}

func (s *BuildkitClaim) ensureBuildkitd(rc *RunContext) error {
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
	switch {
	case err == nil:
		if inspect.State.Running {
			return nil
		}

		logger.V(1).Info("starting existing buildkitd container", "container", buildkitdContainerName)
		if err := cli.ContainerStart(ctx, inspect.ID, dockercontainer.StartOptions{}); err != nil {
			return fmt.Errorf("failed to start buildkitd container: %w", err)
		}

		return nil
	case !dockerclient.IsErrNotFound(err):
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
	resources := container.Resources{}

	if s.opts.MemoryLimit != "" {
		if strings.HasSuffix(s.opts.MemoryLimit, "%") {
			memLimitPercentage, err := strconv.ParseFloat(strings.TrimSuffix(s.opts.MemoryLimit, "%"), 64)
			if err != nil {
				return err
			}

			resources.Memory = int64(float64(totalMem) * memLimitPercentage / 100)
			resources.MemorySwap = resources.Memory
		} else {
			memLimit, err := resource.ParseQuantity(s.opts.MemoryLimit)
			if err != nil {
				return fmt.Errorf("failed to parse memory limit %q: %w", s.opts.MemoryLimit, err)
			}

			resources.Memory = memLimit.Value()
			resources.MemorySwap = resources.Memory
		}
	}

	if s.opts.CPULimit != "" {
		if strings.HasSuffix(s.opts.CPULimit, "%") {
			cpuLimitPercentage, err := strconv.ParseFloat(strings.TrimSuffix(s.opts.CPULimit, "%"), 64)
			if err != nil {
				return err
			}

			resources.NanoCPUs = int64(float64(numCPU) * cpuLimitPercentage / 100 * 1e9)
		} else {
			cpuLimit, err := resource.ParseQuantity(s.opts.CPULimit)
			if err != nil {
				return fmt.Errorf("failed to parse cpu limit %q: %w", s.opts.CPULimit, err)
			}

			resources.NanoCPUs = cpuLimit.MilliValue() * 1e6
		}
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
