package runtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/distribution/reference"
	"github.com/docker/cli/cli/config"
	clitypes "github.com/docker/cli/cli/config/types"
	"github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	imagetypes "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/strslice"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/go-logr/logr"
	"github.com/moby/moby/pkg/jsonmessage"
	"github.com/moby/moby/registry"
	"github.com/moby/term"
	"golang.org/x/sync/errgroup"
	"k8s.io/utils/strings/slices"
)

type dockerOption func(*docker)

func WithLogger(logger logr.Logger) func(*docker) {
	return func(d *docker) {
		d.logger = logger
	}
}

func WithContext(ctx context.Context) func(*docker) {
	return func(d *docker) {
		d.ctx = ctx
	}
}

func WithPullImageWriter(w io.Writer) func(*docker) {
	return func(d *docker) {
		termFd, isTerm := term.GetFdInfo(w)
		d.pullWriter = w
		d.termFd = termFd
		d.isTerm = isTerm
	}
}

type docker struct {
	client     *dockerclient.Client
	logger     logr.Logger
	self       *types.ContainerJSON
	pullWriter io.Writer
	termFd     uintptr
	isTerm     bool
	ctx        context.Context
}

func NewDocker(client *dockerclient.Client, opts ...dockerOption) *docker {
	d := &docker{
		client:     client,
		ctx:        context.Background(),
		logger:     logr.Discard(),
		pullWriter: io.Discard,
	}

	for _, o := range opts {
		o(d)
	}

	hostname, _ := os.Hostname()
	var self *types.ContainerJSON
	s, err := client.ContainerInspect(d.ctx, hostname)
	if err == nil {
		self = &s
	}

	d.self = self

	return d
}

func (d *docker) DeletePod(ctx context.Context, pod *Pod) error {
	wg := new(errgroup.Group)
	for _, container := range pod.Status.Containers {
		containerId := container.ContainerID
		wg.Go(func() error {
			return d.resetContainer(ctx, containerId)
		})
	}

	return wg.Wait()
}

func (d *docker) resetContainer(ctx context.Context, containerID string) error {
	_ = d.client.ContainerStop(ctx, containerID, dockercontainer.StopOptions{})
	return d.client.ContainerRemove(ctx, containerID, dockercontainer.RemoveOptions{})
}

func (d *docker) CreatePod(ctx context.Context, pod *Pod, stdin io.Reader, stdout, stderr io.Writer) (Await, error) {
	for _, container := range pod.Spec.InitContainers {
		createResponse, err := d.createContainer(ctx, pod, container)
		if err != nil {
			return nil, fmt.Errorf("failed to create init container %s: %w", container.Name, err)
		}

		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()

		waitC, _ := d.client.ContainerWait(ctx, createResponse.ID, "next-exit")
		spec, err := d.startContainer(ctx, createResponse.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to create init container %s: %w", container.Name, err)
		}

		await := <-waitC
		if await.StatusCode > 0 {
			return nil, fmt.Errorf("init container exit code > 0: %s", container.Name)
		}

		pod.Status.InitContainers = append(pod.Status.Containers, ContainerStatus{
			ContainerID: spec.ID,
			Name:        container.Name,
		})
	}

	wg := errgroup.Group{}

	if len(pod.Spec.Containers) != 1 {
		return nil, errors.New("exactly one container is required")
	}

	container := pod.Spec.Containers[0]

	pullImage := false
	switch container.ImagePullPolicy {
	case PullImagePolicyAlways:
		pullImage = true
	case PullImagePolicyMissing:
		has, err := d.hasImage(ctx, container.Image)
		if err != nil {
			return nil, err
		}

		pullImage = !has
	case PullImagePolicyNever:
		pullImage = false
	}

	if pullImage {
		if err := d.pullImage(ctx, container.Image); err != nil {
			return nil, fmt.Errorf("failed to pull image: %w", err)
		}
	}

	createResponse, err := d.createContainer(ctx, pod, container)
	if err != nil {
		return nil, fmt.Errorf("failed to create container %s: %w", container.Name, err)
	}

	waitC, _ := d.client.ContainerWait(ctx, createResponse.ID, "next-exit")

	streams, err := d.client.ContainerAttach(ctx, createResponse.ID, dockercontainer.AttachOptions{
		Stdout: stdout != nil,
		Stderr: stderr != nil,
		Stdin:  stdin != nil,
		Stream: true,
	})

	if err != nil {
		return nil, fmt.Errorf("container attach failed: %w", err)
	}

	spec, err := d.startContainer(ctx, createResponse.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to start container %s: %w", container.Name, err)
	}

	var addr string
	for _, netAdapter := range spec.NetworkSettings.Networks {
		if netAdapter.IPAddress != "" {
			addr = netAdapter.IPAddress
			break
		}
	}

	pod.Status.Containers = append(pod.Status.Containers, ContainerStatus{
		ContainerID: spec.ID,
		ContainerIP: addr,
		Name:        container.Name,
	})

	wg.Go(func() error {
		_, err = stdcopy.StdCopy(stdout, stderr, streams.Reader)
		if err != nil {
			return fmt.Errorf("demux container streams failed: %w", err)
		}

		return nil
	})

	if stdin != nil {
		wg.Go(func() (err error) {
			_, err = io.Copy(streams.Conn, stdin)

			defer func() {
				if e := streams.CloseWrite(); e != nil {
					err = fmt.Errorf("could not send eof: %w", e)
				}
			}()

			if errors.Is(err, io.ErrClosedPipe) {
				return nil
			}

			if err != nil {
				err = fmt.Errorf("write stdin stream failed: %w", err)
			}

			return err
		})
	}

	wg.Go(func() error {
		await := <-waitC
		if await.StatusCode > 0 {
			return &Result{
				ExitCode: int(await.StatusCode),
			}
		}

		return nil
	})

	return &await{
		wg:      &wg,
		streams: streams,
	}, nil
}

type await struct {
	streams types.HijackedResponse
	wg      *errgroup.Group
}

func (a *await) Wait() error {
	defer a.streams.Close()
	return a.wg.Wait()

}

type Result struct {
	ExitCode int
}

func (e *Result) Error() string {
	return fmt.Sprintf("container terminated with code %d", e.ExitCode)
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

func (d *docker) hasImage(ctx context.Context, image string) (bool, error) {
	images, err := d.client.ImageList(ctx, imagetypes.ListOptions{})
	if err != nil {
		return false, err
	}

	for _, img := range images {
		if slices.Contains(img.RepoTags, image) {
			return true, nil
		}
	}

	return false, nil
}

func (d *docker) pullImage(ctx context.Context, image string) error {
	ref, err := reference.ParseNormalizedNamed(image)
	if err != nil {
		return err
	}

	configFile := config.LoadDefaultConfigFile(os.Stderr)

	encodedAuth, err := encodedAuth(ref, configFile)
	if err != nil {
		return err
	}

	r, err := d.client.ImagePull(ctx, image, imagetypes.PullOptions{
		RegistryAuth: encodedAuth,
	})
	if err != nil {
		return err
	}

	defer r.Close()
	jsonmessage.DisplayJSONMessagesStream(r, d.pullWriter, d.termFd, d.isTerm, nil)
	return err
}

func (d *docker) getPwd() (string, error) {
	pwd, err := os.Getwd()
	if err != nil {
		return pwd, err
	}

	/*if d.self != nil {
		return pwd, nil
	}*/

	return filepath.Join("/__rootfs", pwd), nil
}

func (d *docker) createContainer(ctx context.Context, pod *Pod, container ContainerSpec) (*dockercontainer.CreateResponse, error) {
	var pwd string

	if container.PWD == "" {
		var err error
		pwd, err = d.getPwd()
		if err != nil {
			return nil, err
		}
	} else {
		pwd = container.PWD
	}

	g, err := os.Getgroups()
	if err != nil {
		return nil, fmt.Errorf("failed to get user groups: %w", err)
	}

	var groups []string
	for _, group := range g {
		groups = append(groups, fmt.Sprintf("%d", group))
	}

	containerConfig := dockercontainer.Config{
		Image:      container.Image,
		StdinOnce:  container.Stdin,
		OpenStdin:  container.Stdin,
		Entrypoint: strslice.StrSlice(container.Command),
		Cmd:        strslice.StrSlice(container.Args),
		Env:        container.Env,
		WorkingDir: pwd,
		User:       fmt.Sprintf("%d:%d", os.Geteuid(), os.Getgid()),
	}

	hostConfig := dockercontainer.HostConfig{
		RestartPolicy: d.getRestartPolicy(container.RestartPolicy),
		GroupAdd:      groups,
		LogConfig:     dockercontainer.LogConfig{
			//	Type: "none",
		},
	}

	netConfig := network.NetworkingConfig{
		EndpointsConfig: make(map[string]*network.EndpointSettings),
	}

	mounts := []mount.Mount{}

	for _, volume := range pod.Spec.Volumes {
		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: volume.HostPath,
			Target: volume.Path,
		})
	}

	if d.self != nil {
		for _, m := range d.self.Mounts {
			mounts = append(mounts, mount.Mount{
				Type:     m.Type,
				Source:   m.Source,
				Target:   m.Destination,
				ReadOnly: !m.RW,
			})
		}
		mounts = append(mounts, d.self.HostConfig.Mounts...)

		for k := range d.self.NetworkSettings.Networks {
			netConfig.EndpointsConfig[k] = &network.EndpointSettings{
				NetworkID: k,
			}
		}
	}

	mounts = append(mounts, mount.Mount{
		Type:   mount.TypeBind,
		Source: "/",
		Target: "/__rootfs",
	})

	hostConfig.Mounts = mounts

	d.logger.V(1).Info("create new container", "container-spec", containerConfig, "host-config", hostConfig, "network-config", netConfig)
	cont, err := d.client.ContainerCreate(ctx, &containerConfig, &hostConfig, &netConfig, nil, fmt.Sprintf("%s-%s", pod.Name, container.Name))

	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	return &cont, err
}

func (d *docker) startContainer(ctx context.Context, containerID string) (*types.ContainerJSON, error) {
	err := d.client.ContainerStart(ctx, containerID, dockercontainer.StartOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	specs, err := d.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}

	d.logger.V(1).Info("container inspect", "container-id", containerID, "container-inspect", specs)
	return &specs, nil
}

func (d *docker) getRestartPolicy(policy RestartPolicy) dockercontainer.RestartPolicy {
	switch policy {
	case RestartPolicyAlways:
		return dockercontainer.RestartPolicy{
			Name: dockercontainer.RestartPolicyAlways,
		}
	case RestartPolicyOnFailure:
		return dockercontainer.RestartPolicy{
			Name: dockercontainer.RestartPolicyOnFailure,
		}
	default:
		return dockercontainer.RestartPolicy{
			Name: dockercontainer.RestartPolicyDisabled,
		}
	}
}
