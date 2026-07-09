package run

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	cruntime "github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/internal/setup/containerdsetup"
	"github.com/raffis/rageta/internal/setup/dockersetup"
	"github.com/raffis/rageta/internal/setup/flagset"
	"github.com/spf13/pflag"
)

type containerRuntime string

var (
	containerRuntimeDocker     containerRuntime = "docker"
	containerRuntimeContainerd containerRuntime = "containerd"
)

func (d containerRuntime) String() string {
	return string(d)
}

func NewContainerRuntimeOptions() ContainerRuntimeOptions {
	return ContainerRuntimeOptions{
		ContainerRuntime:  containerRuntimeDocker.String(),
		ContainerdOptions: containerdsetup.NewOptions(),
	}
}

type ContainerRuntimeOptions struct {
	ContainerRuntime  string
	DockerOptions     dockersetup.Options
	DockerQuiet       bool
	ContainerdOptions containerdsetup.Options
}

func (s ContainerRuntimeOptions) Build() Task {
	return &ContainerRuntime{
		opts: s,
	}
}

func (s *ContainerRuntimeOptions) BindFlags(flags flagset.Interface) {
	flags.StringVarP(&s.ContainerRuntime, "container-runtime", "", s.ContainerRuntime, "Container runtime. One of docker, containerd.")

	dockerFlags := pflag.NewFlagSet("Docker", pflag.ExitOnError)
	dockerFlags.BoolVarP(&s.DockerQuiet, "docker-quiet", "q", false, "Suppress the docker pull output.")
	s.DockerOptions.BindFlags(dockerFlags)
	flags.AddFlagSet(dockerFlags)

	containerdFlags := pflag.NewFlagSet("Containerd", pflag.ExitOnError)
	s.ContainerdOptions.BindFlags(containerdFlags)
	flags.AddFlagSet(containerdFlags)
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
	case containerRuntimeContainerd.String():
		logger.V(1).Info("configuring containerd runtime",
			"container", s.opts.ContainerdOptions.Container,
			"address", s.opts.ContainerdOptions.Address,
			"namespace", s.opts.ContainerdOptions.Namespace,
			"snapshotter", s.opts.ContainerdOptions.Snapshotter,
			"cni", s.opts.ContainerdOptions.CNI,
		)

		return cruntime.NewContainerd(
			cruntime.WithContainerdContainer(s.opts.ContainerdOptions.Container),
			cruntime.WithContainerdDockerContext(s.opts.ContainerdOptions.DockerContext),
			cruntime.WithContainerdCtrAddress(s.opts.ContainerdOptions.Address),
			cruntime.WithContainerdNamespace(s.opts.ContainerdOptions.Namespace),
			cruntime.WithContainerdSnapshotter(s.opts.ContainerdOptions.Snapshotter),
			cruntime.WithContainerdCNI(s.opts.ContainerdOptions.CNI),
			cruntime.WithContainerdLogger(logger),
		), nil
	default:
		return nil, fmt.Errorf("unknown container runtime: %s", s.opts.ContainerRuntime)
	}
}
