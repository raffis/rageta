package run

import (
	"errors"
	"net/url"

	cruntime "github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/internal/setup/flagset"
)

func NewContainerRuntimeOptions() ContainerRuntimeOptions {
	return ContainerRuntimeOptions{
		Address: "docker-container://rageta-buildkitd",
	}
}

type ContainerRuntimeOptions struct {
	Address string `env:"CONTAINERD_ADDRESS"`
}

func (s ContainerRuntimeOptions) Build() Task {
	return &ContainerRuntime{
		opts: s,
	}
}

func (s *ContainerRuntimeOptions) BindFlags(flags flagset.Interface) {
	flags.StringVar(&s.Address, "containerd-address", s.Address, "containerd grpc socket address. Also supports docker-container:// which proxies via the docker api.")
}

type ContainerRuntime struct {
	opts ContainerRuntimeOptions
}

type ContainerRuntimeContext struct {
	Driver cruntime.Interface
}

func (s *ContainerRuntime) Label() string {
	return "Connecting to container runtime"
}

func (s *ContainerRuntime) Run(rc *RunContext, next Next) error {
	u, err := url.Parse(s.opts.Address)
	if err != nil {
		return err
	}

	if u.Scheme != "docker-container" {
		return errors.New("only docker-container:// is supported for containerd")
	}

	cri := cruntime.NewDockerContainerd(
		cruntime.WithDockerContainerName(u.Host),
	)

	rc.ContainerRuntime.Driver = cri
	return next(rc)
}
