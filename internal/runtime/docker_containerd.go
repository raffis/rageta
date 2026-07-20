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

	"github.com/raffis/rageta/internal/utils"
)

type dockerContainerdOption func(*dockerContainerd)

func WithDockerContainerName(name string) dockerContainerdOption {
	return func(r *dockerContainerd) {
		r.container = name
	}
}
func WithContainerdCtrAddress(address string) dockerContainerdOption {
	return func(r *dockerContainerd) {
		if address != "" {
			r.address = address
		}
	}
}

func WithContainerdNamespace(namespace string) dockerContainerdOption {
	return func(r *dockerContainerd) {
		if namespace != "" {
			r.namespace = namespace
		}
	}
}

func WithContainerdSnapshotter(name string) dockerContainerdOption {
	return func(r *dockerContainerd) {
		if name != "" {
			r.snapshotter = name
		}
	}
}

type dockerContainerd struct {
	container   string
	address     string
	namespace   string
	snapshotter string
}

func NewDockerContainerd(opts ...dockerContainerdOption) *dockerContainerd {
	r := &dockerContainerd{
		container:   "rageta-buildkitd",
		address:     "/run/containerd/containerd.sock",
		namespace:   "buildkit",
		snapshotter: "overlayfs",
	}

	for _, o := range opts {
		o(r)
	}

	return r
}

func (r *dockerContainerd) remoteCmd(ctx context.Context, tty bool, name string, args ...string) *exec.Cmd {
	if r.container == "" {
		return exec.CommandContext(ctx, name, args...)
	}

	dockerArgs := []string{}
	execFlags := "-i"
	if tty {
		execFlags = "-it"
	}
	dockerArgs = append(dockerArgs, "exec", execFlags, r.container, name)
	dockerArgs = append(dockerArgs, args...)

	return exec.CommandContext(ctx, "docker", dockerArgs...)
}

// ctrCmd builds an *exec.Cmd invoking `ctr <args>`. See remoteCmd for tty.
func (r *dockerContainerd) ctrCmd(ctx context.Context, tty bool, args ...string) *exec.Cmd {
	ctrArgs := append([]string{"--address", r.address, "--namespace", r.namespace}, args...)
	return r.remoteCmd(ctx, tty, "ctr", ctrArgs...)
}

// run executes cmd, returning its combined stderr (or the exec error) on failure.
func (r *dockerContainerd) run(cmd *exec.Cmd) error {
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
func (r *dockerContainerd) output(ctx context.Context, args ...string) (string, error) {
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

func (r *dockerContainerd) hasImage(ctx context.Context, image string) (bool, error) {
	out, err := r.output(ctx, "image", "ls", "-q")
	if err != nil {
		return false, fmt.Errorf("failed to list images: %w", err)
	}

	return containsLine(out, image), nil
}

func (r *dockerContainerd) taskRunning(ctx context.Context, id string) (bool, error) {
	out, err := r.output(ctx, "task", "ls", "-q")
	if err != nil {
		return false, fmt.Errorf("failed to list tasks: %w", err)
	}

	return containsLine(out, id), nil
}

// lookupIP finds the IP assigned to id's CNI-networked interface by scanning
// the host-local IPAM plugin's own allocation files under
// /var/lib/cni/networks/<network>/<ip>, whose first line records the exact
// container id CNI knows it by (<namespace>-<id>) and whose filename is the IP.
func (r *dockerContainerd) lookupIP(ctx context.Context, id string) (string, error) {
	needle := r.namespace + "-" + id
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

func (r *dockerContainerd) waitForIP(ctx context.Context, id string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)

	for {
		ip, err := r.lookupIP(ctx, id)
		if err != nil {
			return "", err
		}

		if ip != "" {
			return ip, nil
		}

		if time.Now().After(deadline) {
			return "", fmt.Errorf("timed out waiting for IP for %s", id)
		}

		time.Sleep(100 * time.Millisecond)
	}
}

func (r *dockerContainerd) ensureImage(ctx context.Context, spec ContainerSpec, stderr io.Writer) error {
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

	cmd := r.ctrCmd(ctx, false, "image", "pull", "--snapshotter", r.snapshotter, spec.Image)
	cmd.Stdout = stderr

	if err := r.run(cmd); err != nil {
		return fmt.Errorf("failed to pull image `%s`: %w", spec.Image, err)
	}

	return nil
}

func (r *dockerContainerd) runArgs(spec ContainerSpec, id string, detach bool) []string {
	args := []string{"run"}
	if detach {
		args = append(args, "--detach")
	} else {
		args = append(args, "--rm")
	}

	args = append(args, "--cgroup", "")

	if r.snapshotter != "" {
		args = append(args, "--snapshotter", r.snapshotter)
	}

	args = append(args, "--cni")
	args = append(args, "--mount", "type=bind,src=/etc/resolv.conf,dst=/etc/resolv.conf,options=rbind:ro")

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

	args = append(args, spec.Image, id)
	args = append(args, spec.Command...)
	args = append(args, spec.Args...)

	return args
}

func (r *dockerContainerd) Create(ctx context.Context, container *Container, stdin io.Reader, stdout, stderr io.Writer) (Await, error) {
	id := utils.RandString(10)
	if err := r.ensureImage(ctx, container.Spec, stderr); err != nil {
		return nil, fmt.Errorf("failed to pull image %s: %w", container.Spec.Image, err)
	}

	cmd := r.ctrCmd(ctx, container.Spec.TTY, r.runArgs(container.Spec, id, false)...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	containerIP, err := r.waitForIP(ctx, id, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to determine net iface ip addr: %w", err)
	}

	container.Status = ContainerStatus{
		ContainerID: id,
		Name:        container.Name,
		ContainerIP: containerIP,
	}

	return &containerdAwait{cmd: cmd}, nil
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
				return &result{exitCode: code}
			}

			return nil
		}

		return err
	}
}

func (r *dockerContainerd) Delete(ctx context.Context, container *Container, timeout time.Duration) error {
	_ = r.run(r.ctrCmd(ctx, false, "task", "kill", "-s", "SIGTERM", container.Status.ContainerID))

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		running, err := r.taskRunning(ctx, container.Status.ContainerID)
		if err != nil || !running {
			break
		}

		time.Sleep(200 * time.Millisecond)
	}

	_ = r.run(r.ctrCmd(ctx, false, "task", "kill", "-s", "SIGKILL", container.Status.ContainerID))
	_ = r.run(r.ctrCmd(ctx, false, "task", "rm", "-f", container.Status.ContainerID))
	_ = r.run(r.ctrCmd(ctx, false, "container", "rm", container.Status.ContainerID))
	return nil
}
