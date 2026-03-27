package run

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"github.com/raffis/rageta/internal/dockersetup"
	"github.com/raffis/rageta/internal/kubesetup"
	cruntime "github.com/raffis/rageta/internal/runtime"
	"github.com/spf13/pflag"
	"k8s.io/client-go/kubernetes"
)

type containerRuntime string

var (
	containerRuntimeDocker     containerRuntime = "docker"
	containerRuntimeKubernetes containerRuntime = "kubernetes"
)

func (d containerRuntime) String() string {
	return string(d)
}

func NewContainerRuntimeOptions() ContainerRuntimeOptions {
	return ContainerRuntimeOptions{
		ContainerRuntime: electDefaultContainerRuntime().String(),
		KubeOptions:      kubesetup.DefaultOptions(),
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

func electDefaultContainerRuntime() containerRuntime {
	docker, _ := isPossiblyInsideDocker()

	switch {
	case docker:
		return containerRuntimeDocker
		//case isPossiblyInsideKube():
		//		return containerRuntimeKubernetes
	}

	return containerRuntimeDocker
}

func isPossiblyInsideDocker() (bool, error) {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true, nil
	} else {
		return false, err
	}
}

func (s ContainerRuntimeOptions) BindFlags(flags *pflag.FlagSet) {
	flags.StringVarP(&s.ContainerRuntime, "container-runtime", "", s.ContainerRuntime, "Container runtime. One of [docker].")

	dockerFlags := pflag.NewFlagSet("docker", pflag.ExitOnError)
	dockerFlags.BoolVarP(&s.DockerQuiet, "docker-quiet", "q", false, "Suppress the docker pull output.")
	s.DockerOptions.BindFlags(dockerFlags)
	flags.AddFlagSet(dockerFlags)

	kubeFlags := pflag.NewFlagSet("kube", pflag.ExitOnError)
	s.KubeOptions.BindFlags(kubeFlags)
	flags.AddFlagSet(kubeFlags)
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
	case containerRuntimeKubernetes.String():
		if s.opts.KubeOptions == nil {
			return nil, errors.New("kubernetes options not set")
		}
		config, err := s.opts.KubeOptions.ToRESTConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to create kube client: %w", err)
		}
		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			return nil, err
		}
		return cruntime.NewKubernetes(clientset.CoreV1()), nil
	default:
		return nil, fmt.Errorf("unknown container runtime: %s", s.opts.ContainerRuntime)
	}
}
