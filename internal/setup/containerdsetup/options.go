package containerdsetup

import (
	"github.com/raffis/rageta/internal/setup/flagset"
)

const (
	// DefaultAddress is the default containerd grpc socket, matching ctr's
	// own default.
	DefaultAddress = "/run/containerd/containerd.sock"
	// DefaultNamespace is the containerd namespace rageta uses for its pods.
	// This matches buildkitd's own default containerd-worker namespace
	// (defaultContainerdNamespace in buildkitd), so images buildkit produces
	// are visible to rageta without any extra configuration. Override with
	// --containerd-namespace if buildkitd was started with
	// --containerd-worker-namespace set to something else.
	DefaultNamespace = "buildkit"
	// DefaultSnapshotter is the snapshotter used to unpack images.
	DefaultSnapshotter = "overlayfs"
)

// Options configure how rageta invokes the `ctr` CLI to drive containerd.
// Commands are shelled out to `ctr` rather than using containerd's Go client
// library directly: that library expects to run co-located with containerd's
// own filesystem (it does local mount/unpack work for images, not just gRPC
// calls), which breaks the moment rageta runs anywhere else - including from
// macOS, or against a containerd instance living inside a docker container.
// `ctr` doesn't have that problem since it always runs where containerd's
// filesystem actually is.
type Options struct {
	// Container is the name of a running docker container to exec `ctr`
	// commands into, e.g. "bk". Leave empty to run `ctr` directly on the
	// local host (rageta co-located with containerd).
	Container string `env:"CONTAINERD_CONTAINER"`
	// DockerContext is an optional docker CLI context used for the `docker
	// exec` call when Container is set.
	DockerContext string `env:"CONTAINERD_DOCKER_CONTEXT"`
	// Address is the containerd grpc socket path as seen from wherever `ctr`
	// actually runs (i.e. inside Container, if set).
	Address string `env:"CONTAINERD_ADDRESS"`
	// Namespace is the containerd namespace used for every ctr invocation.
	Namespace string `env:"CONTAINERD_NAMESPACE"`
	// Snapshotter is the snapshotter used to unpack/run images.
	Snapshotter string `env:"CONTAINERD_SNAPSHOTTER"`
	// CNI controls whether `ctr run --cni` is used, attaching containers to
	// containerd's CNI network so they get an IP and can reach each other -
	// the same thing the docker runtime gets for free via its default bridge
	// network. Defaults to true for that reason; disable it if no CNI network
	// is configured (see net.conflist) or containers genuinely don't need
	// network access.
	CNI bool `env:"CONTAINERD_CNI"`
}

func NewOptions() Options {
	return Options{
		Address:     DefaultAddress,
		Namespace:   DefaultNamespace,
		Snapshotter: DefaultSnapshotter,
		CNI:         true,
	}
}

// BindFlags registers flags for driving containerd via `ctr`.
func (o *Options) BindFlags(flags flagset.Interface) {
	flags.StringVar(&o.Container, "containerd-container", o.Container, "Name of a running docker container to exec `ctr` commands into (e.g. the container running buildkitd+containerd). Leave empty to run `ctr` directly on the local host.")
	flags.StringVar(&o.DockerContext, "containerd-docker-context", o.DockerContext, "Docker CLI context to use for --containerd-container, if not the current one.")
	flags.StringVar(&o.Address, "containerd-address", o.Address, "containerd grpc socket address, as seen from wherever `ctr` runs.")
	flags.StringVar(&o.Namespace, "containerd-namespace", o.Namespace, "containerd namespace used for rageta pods.")
	flags.StringVar(&o.Snapshotter, "containerd-snapshotter", o.Snapshotter, "containerd snapshotter used to unpack images.")
	flags.BoolVar(&o.CNI, "containerd-cni", o.CNI, "Attach containers to containerd's CNI network so they get an IP and can reach each other, same as docker's default bridge network. Enabled by default; set to false if no CNI network is configured.")
}
