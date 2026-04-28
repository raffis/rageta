package run

import (
	"github.com/moby/buildkit/client"
	cruntime "github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/internal/setup/buildkitsetup"
	"github.com/raffis/rageta/internal/setup/flagset"
	"github.com/spf13/pflag"
)

type BuildkitOptions struct {
	BuildkitOptions buildkitsetup.Options
	CacheImports    []string
	CacheExports    []string
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
	}
}

func (s *BuildkitOptions) BindFlags(flags flagset.Interface) {
	buildkitFlags := pflag.NewFlagSet("buildkit", pflag.ExitOnError)
	buildkitFlags.StringArrayVarP(&s.CacheImports, "cache-from", "", s.CacheImports, "Import build cache, e.g. type=registry,ref=example.com/foo/bar, or type=local,src=path/to/dir")
	buildkitFlags.StringArrayVarP(&s.CacheExports, "cache-to", "", s.CacheExports, "Export build cache, e.g. type=registry,ref=example.com/foo/bar, or type=local,dest=path/to/dir")
	buildkitFlags.BoolVar(&s.NoCache, "no-cache", s.NoCache, "Disable cache for all the vertices")
	s.BuildkitOptions.BindFlags(buildkitFlags)
	flags.AddFlagSet(buildkitFlags)
}

type Buildkit struct {
	opts BuildkitOptions
}

type BuildkitContext struct {
	Client       *client.Client
	CacheImports []client.CacheOptionsEntry
	CacheExports []client.CacheOptionsEntry
	NoCache      bool
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

	rc.Buildkit.Client = c
	rc.Buildkit.CacheImports = cacheImports
	rc.Buildkit.CacheExports = cacheExports
	rc.Buildkit.NoCache = s.opts.NoCache

	return next(rc)
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
