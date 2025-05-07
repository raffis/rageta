package runtime

import (
	"context"
	"io"
)

type Interface interface {
	//Watch(ctx context.Context, pod *Pod) chan Event
	CreatePod(ctx context.Context, pod *Pod, stdin io.Reader, stdout, stderr io.Writer) (Await, error)
	DeletePod(ctx context.Context, pod *Pod) error
}

type Await interface {
	Wait() error
}

type Pod struct {
	Name   string
	Spec   PodSpec
	Status PodStatus
}

type PodSpec struct {
	Containers     []ContainerSpec
	InitContainers []ContainerSpec
}

type Volume struct {
	Name     string
	Path     string
	HostPath string
}

type PodStatus struct {
	PodIP          string
	Containers     []ContainerStatus
	InitContainers []ContainerStatus
}

type ContainerSpec struct {
	Name            string
	Args            []string
	Command         []string
	Image           string
	ImagePullPolicy PullImagePolicy
	Stdin           bool
	TTY             bool
	Env             []string
	Uid             *int
	Guid            *int
	PWD             string
	RestartPolicy   RestartPolicy
	Volumes         []Volume
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

type RestartPolicy string

var (
	RestartPolicyNever     RestartPolicy = "Never"
	RestartPolicyOnFailure RestartPolicy = "OnFailure"
	RestartPolicyAlways    RestartPolicy = "Always"
)

type EventType string

var (
	EventTypeError   EventType = "error"
	EventTypeStart   EventType = "start"
	EventTypeRestart EventType = "restart"
	EventTypeExit    EventType = "exit"
	EventTypeDelete  EventType = "delete"
)

type Event struct {
	Container string
	ExitCode  int
	Type      EventType
	Error     error
}
