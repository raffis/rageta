package processor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/internal/utils"
	"github.com/raffis/rageta/internal/xio"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"

	"github.com/moby/buildkit/client"
)

func WithService(defaultPullPolicy runtime.PullImagePolicy, driver runtime.Interface, buildkit *client.Client, outputFactory OutputFactory, teardown chan Teardown) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Service == nil {
			return nil
		}

		return &Service{
			step:              *spec.Service,
			stepName:          spec.Name,
			driver:            driver,
			defaultPullPolicy: defaultPullPolicy,
			teardown:          teardown,
		}
	}
}

type Service struct {
	stepName          string
	step              v1beta1.ServiceStep
	driver            runtime.Interface
	defaultPullPolicy runtime.PullImagePolicy
	teardown          chan Teardown
}

func (s *Service) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		run := s.step.DeepCopy()
		pod := &runtime.Pod{
			Name: fmt.Sprintf("rageta-%s-%s-%s", pipeline.ID(), ctx.UniqueID(), utils.RandString(5)),
			Spec: runtime.PodSpec{},
		}

		if err := substitute.Substitute(ctx.ToV1Beta1(), run.Guid, run.Uid); err != nil {
			return ctx, err
		}

		envs := make(map[string]string)
		maps.Copy(envs, ctx.EnvVars.Envs)
		maps.Copy(envs, ctx.SecretVars.Secrets)

		command, args := s.commandArgs(run)

		container := runtime.ContainerSpec{
			Name:            s.stepName,
			Stdin:           ctx.Streams.Stdin != nil || run.Stdin,
			TTY:             run.TTY,
			Image:           run.Image,
			ImagePullPolicy: s.defaultPullPolicy,
			Command:         command,
			Args:            args,
			Env:             envs,
			PWD:             run.WorkingDir,
			RestartPolicy:   runtime.RestartPolicy(run.RestartPolicy),
		}

		if run.Guid != nil {
			guid := run.Guid.IntValue()
			container.Guid = &guid
		}

		if run.Uid != nil {
			uid := run.Uid.IntValue()
			container.Uid = &uid
		}

		for _, vol := range run.VolumeMounts {
			container.Volumes = append(container.Volumes, runtime.Volume{
				Name:     vol.Name,
				HostPath: vol.HostPath,
				Path:     vol.MountPath,
			})
		}

		if ctx.Template.Template != nil {
			if err := substitute.Substitute(ctx.ToV1Beta1(), ctx.Template.Template.Guid, ctx.Template.Template.Uid); err != nil {
				return ctx, err
			}

			ContainerSpec(&container, ctx.Template.Template)
		}

		subst := []any{
			&container.Image,
			container.Args,
			container.Command,
			&container.PWD,
		}

		for i := range container.Volumes {
			subst = append(subst, &container.Volumes[i].HostPath, &container.Volumes[i].Path)
		}

		if err := substitute.Substitute(ctx.ToV1Beta1(), subst...); err != nil {
			return ctx, err
		}

		for i, vol := range container.Volumes {
			srcPath, err := filepath.Abs(vol.HostPath)
			if err != nil {
				return ctx, fmt.Errorf("failed to get absolute path: %w", err)
			}

			container.Volumes[i].HostPath = srcPath
		}

		if run.Stdin && ctx.Streams.Stdin == nil {
			ctx.Streams.Stdin = os.Stdin
		}

		pod.Spec.Containers = []runtime.ContainerSpec{container}

		_, _ = ctx.Events.Dev.Write([]byte(fmt.Sprintf("🐋 starting %s", container.Image) + "\n"))
		fmt.Println(strings.Join(append(command, args...), " "))
		ctx, err := s.exec(ctx, pod)

		if err != nil {
			var exitCode int
			var runtimeErr ExitCode
			if errors.As(err, &runtimeErr) {
				exitCode = runtimeErr.ExitCode()
			}

			return ctx, &ContainerError{
				containerName: pod.Name,
				image:         container.Image,
				exitCode:      exitCode,
				err:           err,
			}
		}

		return next(ctx)
	}, nil
}

func (s *Service) commandArgs(run *v1beta1.ServiceStep) (cmd []string, args []string) {
	script := strings.TrimSpace(run.Script)
	args = run.Args

	if script == "" {
		return run.Command, run.Args
	}

	hasShebang := strings.HasPrefix(script, "#!")
	if hasShebang {
		lines := strings.Split(script, "\n")
		header := lines[0]
		shebang := strings.Split(header, "#!")
		cmd = []string{shebang[1]}
		args = append(args, "-e", "-c", strings.Join(lines[1:], "\n"))
	} else {
		if len(run.Command) == 0 {
			cmd = []string{defaultShell}
		} else {
			cmd = run.Command
		}

		args = append(args, "-e", "-c", script)
	}

	return
}

func (s *Service) exec(ctx StepContext, pod *runtime.Pod) (StepContext, error) {
	if len(pod.Spec.Containers[0].Command) > 0 || len(pod.Spec.Containers[0].Args) > 0 {
		cmd := strings.Join(append(pod.Spec.Containers[0].Command, pod.Spec.Containers[0].Args...), " ")
		w := xio.NewLineWriter(xio.NewPrefixWriter(ctx.Events.Dev, []byte("$ ")))
		w.Write([]byte(cmd))
		w.Flush()
	}

	await, err := s.driver.CreatePod(ctx, pod, ctx.Streams.Stdin,
		io.MultiWriter(append(ctx.Streams.AdditionalStdout, ctx.Streams.Stdout)...),
		io.MultiWriter(append(ctx.Streams.AdditionalStderr, ctx.Streams.Stderr)...),
	)

	if err != nil {
		return ctx, err
	}

	for _, v := range pod.Status.Containers {
		ctx.Containers[v.Name] = v
	}

	done := make(chan error)
	go func() {
		if err := await.Wait(ctx); err != nil {
			done <- err
		}

		done <- nil
	}()

	s.teardown <- func(teardownCtx context.Context, timeout time.Duration) error {
		//In case of Await == v1beta1.AwaitStatusReady we need to delete the container here
		//otherwise we end up with orphaned running containers
		//And in addition if the container here is not deleted the done channel will never be fulfilled as there is nothing which will stop
		//the containers otherwise if the app was started with --no-gc (skip pod deletion)
		if containerStatus, ok := ctx.Containers[s.stepName]; ok {
			err := s.driver.DeletePod(teardownCtx, &runtime.Pod{
				Status: runtime.PodStatus{
					Containers: []runtime.ContainerStatus{containerStatus},
				},
			}, timeout)

			if err != nil {
				return err
			}
		}

		return <-done
	}

	return ctx, err
}
