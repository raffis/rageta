package run

import (
	"context"
	"io"

	"github.com/docker/cli/cli/config"
	"github.com/moby/buildkit/client"
	gwclient "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth/authprovider"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	"github.com/raffis/rageta/internal/processor"
	cruntime "github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/internal/setup/buildkitsetup"
	"github.com/raffis/rageta/internal/setup/flagset"
	"github.com/spf13/pflag"
	"github.com/tonistiigi/fsutil"
)

type BuildkitOptions struct {
	BuildkitOptions buildkitsetup.Options
	CacheImports    []string
	CacheExports    []string
	BuildContext    string
	NoCache         bool
}

func (s BuildkitOptions) Build() Step {
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
	buildkitFlags := pflag.NewFlagSet("buildkit", pflag.ExitOnError)
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
	StatusRouter   *processor.VertexStatusRouter
	CacheImports   []client.CacheOptionsEntry
	GWCacheImports []gwclient.CacheOptionsEntry
	CacheExports   []client.CacheOptionsEntry
	NoCache        bool
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

	statusRouter := processor.NewVertexStatusRouter()

	contextFS, err := fsutil.NewFS(s.opts.BuildContext)
	if err != nil {
		return err
	}

	rc.Buildkit.Client = c
	rc.Buildkit.CacheImports = cacheImports
	rc.Buildkit.GWCacheImports = gwCacheImports
	rc.Buildkit.CacheExports = cacheExports
	rc.Buildkit.NoCache = s.opts.NoCache
	rc.Buildkit.StatusRouter = statusRouter

	buildOpt := client.SolveOpt{
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
			statusRouter.Route(status)
		}
	}()

	var buildErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, buildErr = c.Build(rc.Context, buildOpt, "", func(ctx context.Context, gwc gwclient.Client) (*gwclient.Result, error) {
			rc.Buildkit.GatewayClient = gwc
			return gwclient.NewResult(), next(rc)
		}, ch)
	}()

	<-done
	return buildErr
}

func (s *Buildkit) ensureBuildkitd(rc *RunContext) error {
	return rc.ContainerRuntime.Driver.RunDetached(rc.Context, &cruntime.Pod{
		Name: "rageta",
		Spec: cruntime.PodSpec{
			Containers: []cruntime.ContainerSpec{
				{
					Name:            "buildkitd",
					Image:           "moby/buildkit:latest",
					ImagePullPolicy: cruntime.PullImagePolicyMissing,
					Privileged:      true,
					RestartPolicy:   cruntime.RestartPolicyAlways,
				},
			},
		},
	})
}
