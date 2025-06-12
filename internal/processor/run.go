package processor

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/internal/utils"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithRun(defaultPullPolicy runtime.PullImagePolicy, driver runtime.Interface, outputFactory OutputFactory, teardown chan Teardown) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Run == nil {
			return nil
		}

		return &Run{
			step:              *spec.Run,
			stepName:          spec.Name,
			driver:            driver,
			defaultPullPolicy: defaultPullPolicy,
			teardown:          teardown,
		}
	}
}

const (
	defaultScriptHeader = "#!/bin/sh\nset -e\n"
)

type Run struct {
	stepName          string
	step              v1beta1.RunStep
	driver            runtime.Interface
	defaultPullPolicy runtime.PullImagePolicy
	teardown          chan Teardown
}

func (s *Run) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		run := s.step.DeepCopy()
		pod := &runtime.Pod{
			Name: fmt.Sprintf("%s-%s-%s", SuffixName(s.stepName, ctx.NamePrefix), pipeline.ID(), utils.RandString(5)),
			Spec: runtime.PodSpec{},
		}

		if err := substitute.Substitute(ctx.ToV1Beta1(), run.Guid, run.Uid); err != nil {
			return ctx, err
		}

		envs := make(map[string]string)
		maps.Copy(envs, ctx.Envs)
		maps.Copy(envs, ctx.Secrets)

		command, args := s.commandArgs(run)
		container := runtime.ContainerSpec{
			Name:            s.stepName,
			Stdin:           ctx.Stdin != nil || run.Stdin,
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

		if ctx.Template != nil {
			if err := substitute.Substitute(ctx.ToV1Beta1(), ctx.Template.Guid, ctx.Template.Uid); err != nil {
				return ctx, err
			}

			ContainerSpec(&container, ctx.Template)
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

		for _, vol := range container.Volumes {
			srcPath, err := filepath.Abs(vol.HostPath)
			if err != nil {
				return ctx, fmt.Errorf("failed to get absolute path: %w", err)
			}

			vol.HostPath = srcPath
		}

		if run.Stdin && ctx.Stdin == nil {
			ctx.Stdin = os.Stdin
		}

		pod.Spec.Containers = []runtime.ContainerSpec{container}
		ctx, err := s.exec(ctx, pod)

		if err != nil {
			return ctx, fmt.Errorf("container %s failed: %w", pod.Name, err)
		}

		return next(ctx)
	}, nil
}

func ContainerSpec(container *runtime.ContainerSpec, template *v1beta1.Template) {
	if len(container.Args) == 0 {
		container.Args = template.Args
	}

	if len(container.Command) == 0 {
		container.Command = template.Command
	}

	if container.PWD == "" {
		container.PWD = template.WorkingDir
	}

	if container.Image == "" {
		container.Image = template.Image
	}

	if container.Uid == nil && template.Uid != nil {
		uid := template.Uid.IntValue()
		container.Uid = &uid
	}

	if container.Guid == nil && template.Guid != nil {
		guid := template.Guid.IntValue()
		container.Uid = &guid
	}

	for _, templateVol := range template.VolumeMounts {
		hasVolume := false
		for _, containerVol := range container.Volumes {
			if templateVol.Name == containerVol.Name {
				hasVolume = true
				break
			}
		}

		if !hasVolume {
			container.Volumes = append(container.Volumes, runtime.Volume{
				Name:     templateVol.Name,
				HostPath: templateVol.HostPath,
				Path:     templateVol.MountPath,
			})
		}
	}
}

func (s *Run) commandArgs(run *v1beta1.RunStep) ([]string, []string) {
	script := strings.TrimSpace(run.Script)
	args := run.Args

	if script == "" {
		return run.Command, run.Args
	}

	hasShebang := strings.HasPrefix(script, "#!")

	if !hasShebang {
		script = defaultScriptHeader + script
	}

	header := strings.Split(script, "\n")[0]
	shebang := strings.Split(header, "#!")
	command := []string{shebang[1]}

	return command, append(args, "-c", script)
}

func (s *Run) exec(ctx StepContext, pod *runtime.Pod) (StepContext, error) {
	await, err := s.driver.CreatePod(ctx, pod, ctx.Stdin,
		io.MultiWriter(append(ctx.AdditionalStdout, ctx.Stdout)...),
		io.MultiWriter(append(ctx.AdditionalStderr, ctx.Stderr)...),
	)

	if err != nil {
		return ctx, err
	}

	for _, v := range pod.Status.Containers {
		ctx.Containers[v.Name] = v
	}

	if s.step.Await == v1beta1.AwaitStatusReady {
		done := make(chan error)
		go func() {
			if err := await.Wait(); err != nil {
				done <- err
			}

			done <- nil
		}()

		s.teardown <- func(ctx context.Context) error {
			return <-done
		}
	} else {
		if err := await.Wait(); err != nil {
			return ctx, err
		}
	}

	return ctx, err
}
