package run

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/raffis/rageta/internal/dockersetup"
	"github.com/raffis/rageta/internal/kubesetup"
	cruntime "github.com/raffis/rageta/internal/runtime"
	"github.com/spf13/pflag"
)

type containerRuntime string

var (
	containerRuntimeDocker containerRuntime = "docker"
)

func (d containerRuntime) String() string {
	return string(d)
}

func NewContainerRuntimeOptions() ContainerRuntimeOptions {
	return ContainerRuntimeOptions{
		ContainerRuntime: containerRuntimeDocker.String(),
	}
}

type ContainerRuntimeOptions struct {
	ContainerRuntime string
	DockerOptions    dockersetup.Options
	KubeOptions      *kubesetup.Options
	DockerQuiet      bool
}

func (s ContainerRuntimeOptions) Build() Step {
	return &ContainerRuntime{
		opts: s,
	}
}

func (s ContainerRuntimeOptions) BindFlags(flags *pflag.FlagSet) {
	flags.StringVarP(&s.ContainerRuntime, "container-runtime", "", s.ContainerRuntime, "Container runtime. Only docker is supported.")

	dockerFlags := pflag.NewFlagSet("docker", pflag.ExitOnError)
	dockerFlags.BoolVarP(&s.DockerQuiet, "docker-quiet", "q", false, "Suppress the docker pull output.")
	s.DockerOptions.BindFlags(dockerFlags)
	flags.AddFlagSet(dockerFlags)
}

type ContainerRuntime struct {
	opts ContainerRuntimeOptions
}

type ContainerRuntimeContext struct {
	Driver cruntime.Interface
}

func (s *ContainerRuntime) Run(rc *RunContext, next Next) error {
	driver, err := s.createContainerRuntime(rc.Context, rc.Logging.Logger)
	if err != nil {
		return err
	}

	rc.ContainerRuntime.Driver = driver
	return next(rc)
}

func (s *ContainerRuntime) createContainerRuntime(ctx context.Context, logger logr.Logger) (cruntime.Interface, error) {
	logger.V(3).Info("create container runtime client", "container-runtime", s.opts.ContainerRuntime)

	switch s.opts.ContainerRuntime {
	case containerRuntimeDocker.String():
		s.opts.DockerOptions.Logger = logger
		c, err := s.opts.DockerOptions.Build()
		if err != nil {
			return nil, fmt.Errorf("failed to create docker client: %w", err)
		}
		return cruntime.NewDocker(c,
			cruntime.WithContext(ctx),
			cruntime.WithHidePullOutput(s.opts.DockerQuiet),
			cruntime.WithLogger(logger),
		), nil
	default:
		return nil, fmt.Errorf("unknown container runtime: %s", s.opts.ContainerRuntime)
	}
}
