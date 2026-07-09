package runtime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/go-logr/logr"
)

type containerdOption func(*containerdRuntime)

func WithContainerdContainer(name string) containerdOption {
	return func(r *containerdRuntime) {
		r.container = name
	}
}

func WithContainerdDockerContext(name string) containerdOption {
	return func(r *containerdRuntime) {
		r.dockerContext = name
	}
}

func WithContainerdCtrAddress(address string) containerdOption {
	return func(r *containerdRuntime) {
		if address != "" {
			r.address = address
		}
	}
}

func WithContainerdNamespace(namespace string) containerdOption {
	return func(r *containerdRuntime) {
		if namespace != "" {
			r.namespace = namespace
		}
	}
}

func WithContainerdSnapshotter(name string) containerdOption {
	return func(r *containerdRuntime) {
		if name != "" {
			r.snapshotter = name
		}
	}
}

func WithContainerdCNI(enabled bool) containerdOption {
	return func(r *containerdRuntime) {
		r.cni = enabled
	}
}

func WithContainerdLogger(logger logr.Logger) containerdOption {
	return func(r *containerdRuntime) {
		r.logger = logger
	}
}

// containerdRuntime implements Interface by shelling out to the `ctr` CLI,
// optionally proxied through `docker exec` into a container where containerd
// actually runs. This is deliberately not built on containerd's Go client
// library: that library performs local mount/unpack work assuming it's
// co-located with containerd's own filesystem, which breaks as soon as the
// caller runs anywhere else (a different host, a different container, or
// macOS). `ctr` doesn't have that problem, since it always executes where
// containerd's filesystem actually lives.
type containerdRuntime struct {
	container     string
	dockerContext string
	address       string
	namespace     string
	snapshotter   string
	cni           bool
	logger        logr.Logger
}

func NewContainerd(opts ...containerdOption) *containerdRuntime {
	r := &containerdRuntime{
		address:     "/run/containerd/containerd.sock",
		namespace:   "buildkit",
		snapshotter: "overlayfs",
		logger:      logr.Discard(),
	}

	for _, o := range opts {
		o(r)
	}

	return r
}

// IsContainerdBacked marks containerdRuntime as ContainerdBacked.
func (r *containerdRuntime) IsContainerdBacked() {}

func (r *containerdRuntime) loggerFrom(ctx context.Context) logr.Logger {
	if logger, err := logr.FromContext(ctx); err == nil {
		return logger
	}

	return r.logger
}

// remoteCmd builds an *exec.Cmd for name+args, wrapped in `docker exec -i
// <container> <name> <args>` when r.container is set, so it runs wherever
// containerd's own filesystem actually is. tty must be true when running
// `ctr run --tty ...`: ctr's own tty handling calls console.Current() on its
// stdin, which panics unless that stdin is a real tty fd - `docker exec` only
// provides one when given -t.
func (r *containerdRuntime) remoteCmd(ctx context.Context, tty bool, name string, args ...string) *exec.Cmd {
	if r.container == "" {
		return exec.CommandContext(ctx, name, args...)
	}

	dockerArgs := []string{}
	if r.dockerContext != "" {
		dockerArgs = append(dockerArgs, "--context="+r.dockerContext)
	}

	execFlags := "-i"
	if tty {
		execFlags = "-it"
	}
	dockerArgs = append(dockerArgs, "exec", execFlags, r.container, name)
	dockerArgs = append(dockerArgs, args...)

	return exec.CommandContext(ctx, "docker", dockerArgs...)
}

// ctrCmd builds an *exec.Cmd invoking `ctr <args>`. See remoteCmd for tty.
func (r *containerdRuntime) ctrCmd(ctx context.Context, tty bool, args ...string) *exec.Cmd {
	ctrArgs := append([]string{"--address", r.address, "--namespace", r.namespace}, args...)
	return r.remoteCmd(ctx, tty, "ctr", ctrArgs...)
}

// run executes cmd, returning its combined stderr (or the exec error) on failure.
func (r *containerdRuntime) run(cmd *exec.Cmd) error {
	var stderr bytes.Buffer
	if cmd.Stderr == nil {
		cmd.Stderr = &stderr
	}

	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("%s: %w", msg, err)
		}

		return err
	}

	return nil
}

// output executes cmd and returns its stdout.
func (r *containerdRuntime) output(ctx context.Context, args ...string) (string, error) {
	cmd := r.ctrCmd(ctx, false, args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := r.run(cmd); err != nil {
		return "", err
	}

	return stdout.String(), nil
}

func containsLine(output, needle string) bool {
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == needle {
			return true
		}
	}

	return false
}

func (r *containerdRuntime) hasImage(ctx context.Context, image string) (bool, error) {
	out, err := r.output(ctx, "image", "ls", "-q")
	if err != nil {
		return false, fmt.Errorf("failed to list images: %w", err)
	}

	return containsLine(out, image), nil
}

func (r *containerdRuntime) taskRunning(ctx context.Context, id string) (bool, error) {
	out, err := r.output(ctx, "task", "ls", "-q")
	if err != nil {
		return false, fmt.Errorf("failed to list tasks: %w", err)
	}

	return containsLine(out, id), nil
}

func (r *containerdRuntime) containerExists(ctx context.Context, id string) (bool, error) {
	out, err := r.output(ctx, "container", "ls", "-q")
	if err != nil {
		return false, fmt.Errorf("failed to list containers: %w", err)
	}

	return containsLine(out, id), nil
}

// lookupIP finds the IP assigned to id's CNI-networked interface by scanning
// the host-local IPAM plugin's own allocation files under
// /var/lib/cni/networks/<network>/<ip>, whose first line records the exact
// container id CNI knows it by (<namespace>-<id>) and whose filename is the
// IP. This works without needing any networking tool (ip, ifconfig, ...)
// inside either the target image or wherever containerd runs - both are
// dependencies we can't guarantee.
func (r *containerdRuntime) lookupIP(ctx context.Context, id string) (string, error) {
	needle := r.namespace + "-" + id
	// Substring match, not an exact line match: some CNI plugin builds write
	// these allocation files with CRLF line endings, which would otherwise
	// fail an exact match against the trailing "\r".
	script := fmt.Sprintf("grep -rl %q /var/lib/cni/networks/*/ 2>/dev/null | head -n1", needle)

	cmd := r.remoteCmd(ctx, false, "sh", "-c", script)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := r.run(cmd); err != nil {
		return "", fmt.Errorf("lookupIP %q: %w", needle, err)
	}

	path := strings.TrimSpace(stdout.String())
	if path == "" {
		return "", nil
	}

	return path[strings.LastIndex(path, "/")+1:], nil
}

// waitForIP polls lookupIP until it finds an address or timeout elapses.
// Needed because CNI setup for a foreground `ctr run` happens on the remote
// side, asynchronously with respect to our local exec.Cmd.Start() call.
func (r *containerdRuntime) waitForIP(ctx context.Context, logger logr.Logger, id string, timeout time.Duration) string {
	needle := r.namespace + "-" + id
	deadline := time.Now().Add(timeout)
	attempts := 0

	for {
		attempts++
		ip, err := r.lookupIP(ctx, id)
		if err != nil {
			fmt.Printf("[containerd] lookupIP failed needle=%q attempt=%d error=%v\n", needle, attempts, err)
		}

		if err == nil && ip != "" {
			fmt.Printf("[containerd] found container IP needle=%q ip=%q attempts=%d\n", needle, ip, attempts)
			return ip
		}

		if time.Now().After(deadline) {
			fmt.Printf("[containerd] lookupIP found no match before timeout needle=%q attempts=%d\n", needle, attempts)
			return ""
		}

		time.Sleep(100 * time.Millisecond)
	}
}

// ensureImage pulls image if the pull policy requires it. `ctr run` doesn't
// pull images itself, unlike `docker run`.
func (r *containerdRuntime) ensureImage(ctx context.Context, logger logr.Logger, spec ContainerSpec, stderr io.Writer) error {
	pull := false
	switch spec.ImagePullPolicy {
	case PullImagePolicyAlways:
		pull = true
	case PullImagePolicyMissing:
		has, err := r.hasImage(ctx, spec.Image)
		if err != nil {
			return err
		}
		pull = !has
	case PullImagePolicyNever:
		pull = false
	}

	if !pull {
		return nil
	}

	logger.V(1).Info("pulling image", "image", spec.Image)
	startedAt := time.Now()

	cmd := r.ctrCmd(ctx, false, "image", "pull", "--snapshotter", r.snapshotter, spec.Image)
	cmd.Stdout = stderr
	if err := r.run(cmd); err != nil {
		return fmt.Errorf("failed to pull image `%s`: %w", spec.Image, err)
	}

	logger.V(1).Info("image pulled", "image", spec.Image, "duration", time.Since(startedAt))
	return nil
}

// runArgs builds the arguments for `ctr run`.
func (r *containerdRuntime) runArgs(spec ContainerSpec, id string, detach bool) []string {
	args := []string{"run"}
	if detach {
		args = append(args, "--detach")
	} else {
		args = append(args, "--rm")
	}

	// rageta doesn't set resource limits, so there's no need for ctr to pick
	// a specific cgroup path. Note this alone doesn't fix nested-cgroupv2
	// environments (e.g. running inside a privileged docker container) where
	// the host process is directly attached to cgroup root - that needs a
	// startup-time fix wherever containerd itself runs (move pid 1 into a
	// leaf cgroup and delegate controllers via cgroup.subtree_control).
	args = append(args, "--cgroup", "")

	if r.snapshotter != "" {
		args = append(args, "--snapshotter", r.snapshotter)
	}

	if r.cni {
		args = append(args, "--cni")
		// `ctr run` doesn't bind-mount resolv.conf the way `docker run` does,
		// so without this containers on the CNI network have no working DNS.
		// The source path is resolved wherever containerd/ctr actually runs
		// (inside --containerd-container, if set), not on this process.
		args = append(args, "--mount", "type=bind,src=/etc/resolv.conf,dst=/etc/resolv.conf,options=rbind:ro")
	}

	if spec.TTY {
		args = append(args, "--tty")
	}

	if spec.PWD != "" {
		args = append(args, "--cwd", spec.PWD)
	}

	switch {
	case spec.Uid != nil && spec.Guid != nil:
		args = append(args, "--user", fmt.Sprintf("%d:%d", *spec.Uid, *spec.Guid))
	case spec.Uid != nil:
		args = append(args, "--user", fmt.Sprintf("%d", *spec.Uid))
	}

	if spec.Privileged {
		args = append(args, "--privileged")
	}

	for k, v := range spec.Env {
		args = append(args, "--env", fmt.Sprintf("%s=%s", k, v))
	}

	for _, v := range spec.Volumes {
		mode := "rbind:rw"
		if v.ReadOnly {
			mode = "rbind:ro"
		}
		args = append(args, "--mount", fmt.Sprintf("type=bind,src=%s,dst=%s,options=%s", v.HostPath, v.Path, mode))
	}

	args = append(args, spec.Image, id)
	args = append(args, spec.Command...)
	args = append(args, spec.Args...)

	return args
}

func (r *containerdRuntime) CreatePod(ctx context.Context, pod *Pod, stdin io.Reader, stdout, stderr io.Writer) (Await, error) {
	logger := r.loggerFrom(ctx)
	fmt.Printf("[containerd] CreatePod pod=%q cni=%v container=%q namespace=%q address=%q\n", pod.Name, r.cni, r.container, r.namespace, r.address)

	for _, spec := range pod.Spec.InitContainers {
		if err := r.runInitContainer(ctx, logger, pod, spec); err != nil {
			return nil, err
		}
	}

	if len(pod.Spec.Containers) != 1 {
		return nil, errors.New("exactly one container is required")
	}

	spec := pod.Spec.Containers[0]
	id := fmt.Sprintf("%s-%s", pod.Name, spec.Name)

	if err := r.ensureImage(ctx, logger, spec, stderr); err != nil {
		return nil, fmt.Errorf("failed to create container %s: %w", spec.Name, err)
	}

	cmd := r.ctrCmd(ctx, spec.TTY, r.runArgs(spec, id, false)...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start container %s: %w", spec.Name, err)
	}

	var containerIP string
	if r.cni {
		containerIP = r.waitForIP(ctx, logger, id, 5*time.Second)
		if containerIP == "" {
			fmt.Printf("[containerd] could not determine container IP within timeout container-id=%q\n", id)
		}
	}

	pod.Status.Containers = append(pod.Status.Containers, ContainerStatus{
		ContainerID: id,
		Name:        spec.Name,
		ContainerIP: containerIP,
	})

	return &containerdAwait{cmd: cmd}, nil
}

func (r *containerdRuntime) runInitContainer(ctx context.Context, logger logr.Logger, pod *Pod, spec ContainerSpec) error {
	id := fmt.Sprintf("%s-%s", pod.Name, spec.Name)

	if err := r.ensureImage(ctx, logger, spec, io.Discard); err != nil {
		return fmt.Errorf("failed to create init container %s: %w", spec.Name, err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	cmd := r.ctrCmd(waitCtx, false, r.runArgs(spec, id, false)...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() > 0 {
			return fmt.Errorf("init container exit code > 0: %s", spec.Name)
		}

		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("failed to create init container %s: %s: %w", spec.Name, msg, err)
		}

		return fmt.Errorf("failed to create init container %s: %w", spec.Name, err)
	}

	pod.Status.InitContainers = append(pod.Status.InitContainers, ContainerStatus{
		ContainerID: id,
		Name:        spec.Name,
	})

	return nil
}

type containerdAwait struct {
	cmd *exec.Cmd
}

func (a *containerdAwait) Wait(ctx context.Context) error {
	done := make(chan error, 1)
	go func() {
		done <- a.cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		if err == nil {
			return nil
		}

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if code := exitErr.ExitCode(); code > 0 {
				return &Result{exitCode: code}
			}

			return nil
		}

		return err
	}
}

func (r *containerdRuntime) DeletePod(ctx context.Context, pod *Pod, timeout time.Duration) error {
	for _, container := range pod.Status.Containers {
		r.resetContainer(ctx, container.ContainerID, timeout)
	}

	return nil
}

// resetContainer stops and removes a container, best-effort: containerd state
// left behind after a crash isn't worth failing pipeline teardown over.
func (r *containerdRuntime) resetContainer(ctx context.Context, id string, timeout time.Duration) {
	_ = r.run(r.ctrCmd(ctx, false, "task", "kill", "-s", "SIGTERM", id))

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		running, err := r.taskRunning(ctx, id)
		if err != nil || !running {
			break
		}

		time.Sleep(200 * time.Millisecond)
	}

	_ = r.run(r.ctrCmd(ctx, false, "task", "kill", "-s", "SIGKILL", id))
	_ = r.run(r.ctrCmd(ctx, false, "task", "rm", "-f", id))
	_ = r.run(r.ctrCmd(ctx, false, "container", "rm", id))
}

func (r *containerdRuntime) RunDetached(ctx context.Context, pod *Pod) error {
	logger := r.loggerFrom(ctx)

	if len(pod.Spec.Containers) != 1 {
		return errors.New("exactly one container is required")
	}

	spec := pod.Spec.Containers[0]
	id := fmt.Sprintf("%s-%s", pod.Name, spec.Name)

	running, err := r.taskRunning(ctx, id)
	if err != nil {
		return err
	}
	if running {
		return nil
	}

	exists, err := r.containerExists(ctx, id)
	if err != nil {
		return err
	}
	if exists {
		return r.run(r.ctrCmd(ctx, false, "task", "start", id))
	}

	if err := r.ensureImage(ctx, logger, spec, io.Discard); err != nil {
		return err
	}

	return r.run(r.ctrCmd(ctx, false, r.runArgs(spec, id, true)...))
}
