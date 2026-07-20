package runtime

import (
	"context"
	"fmt"
	"io"
	"time"
)

type Interface interface {
	Create(ctx context.Context, container *Container, stdin io.Reader, stdout, stderr io.Writer) (Await, error)
	Delete(ctx context.Context, container *Container, timeout time.Duration) error
}

type Await interface {
	Wait(ctx context.Context) error
}

type Container struct {
	Name   string
	Spec   ContainerSpec
	Status ContainerStatus
}

type ContainerSpec struct {
	Args            []string
	Command         []string
	Image           string
	ImagePullPolicy PullImagePolicy
	Stdin           bool
	TTY             bool
	Env             map[string]string
	Uid             *int
	Guid            *int
	PWD             string
	Privileged      bool
}

type ContainerStatus struct {
	ContainerID string
	ContainerIP string
	Name        string
	Ready       bool
	Started     bool
	ExitCode    int
}

type PullImagePolicy string

var (
	PullImagePolicyAlways  PullImagePolicy = "Always"
	PullImagePolicyNever   PullImagePolicy = "Never"
	PullImagePolicyMissing PullImagePolicy = "Missing"
)

type result struct {
	exitCode int
}

func (r *result) ExitCode() int {
	return r.exitCode
}

func (r *result) Error() string {
	return fmt.Sprintf("container exit: %d", r.ExitCode)
}
