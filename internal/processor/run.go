package processor

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/internal/utils"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithRun(tee bool, defaultPullPolicy runtime.PullImagePolicy, driver runtime.Interface, outputFactory OutputFactory, teardown chan Teardown) ProcessorBuilder {
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
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		run := s.step.DeepCopy()

		if err := Subst(stepContext.ToV1Beta1(),
			&run.Image,
			run.Args,
			run.Command,
			&run.Script,
			&run.WorkingDir,
		); err != nil {
			return stepContext, err
		}

		pod := &runtime.Pod{
			Name: fmt.Sprintf("%s-%s-%s", PrefixName(stepContext.NamePrefix, s.stepName), pipeline.ID(), utils.RandString(5)),
			Spec: runtime.PodSpec{},
		}

		command, args := s.commandArgs(run)
		container := runtime.ContainerSpec{
			Name:            s.stepName,
			Stdin:           stepContext.Stdin != nil,
			TTY:             run.TTY,
			Image:           run.Image,
			ImagePullPolicy: s.defaultPullPolicy,
			Command:         command,
			Args:            args,
			Env:             envSlice(stepContext.Envs),
			PWD:             run.WorkingDir,
			RestartPolicy:   runtime.RestartPolicy(run.RestartPolicy),
		}

		for _, vol := range run.VolumeMounts {
			srcPath, err := filepath.Abs(vol.SourcePath)
			if err != nil {
				return stepContext, fmt.Errorf("failed to get absolute path: %w", err)
			}

			container.Volumes = append(container.Volumes, runtime.Volume{
				Name:     vol.Name,
				HostPath: srcPath,
				Path:     vol.MountPath,
			})
		}

		s.containerSpec(&container, stepContext.Template)

		pod.Spec.Containers = []runtime.ContainerSpec{container}
		stepContext, err := s.exec(ctx, stepContext, pod)

		if err != nil {
			return stepContext, fmt.Errorf("container %s failed: %w", pod.Name, err)
		}

		return next(ctx, stepContext)
	}, nil
}

func (s *Run) containerSpec(container *runtime.ContainerSpec, template *v1beta1.Template) {
	if template == nil {
		return
	}

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
				HostPath: templateVol.SourcePath,
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

func envSlice(env map[string]string) []string {
	var envs []string
	for k, v := range env {
		envs = append(envs, fmt.Sprintf("%s=%s", k, v))
	}

	return envs
}

func (s *Run) exec(ctx context.Context, stepContext StepContext, pod *runtime.Pod) (StepContext, error) {
	await, err := s.driver.CreatePod(ctx, pod, stepContext.Stdin,
		io.MultiWriter(append(stepContext.AdditionalStdout, stepContext.Stdout)...),
		io.MultiWriter(append(stepContext.AdditionalStderr, stepContext.Stderr)...),
	)

	if err != nil {
		return stepContext, err
	}

	for _, v := range pod.Status.Containers {
		stepContext.Containers[v.Name] = v
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
			return stepContext, err
		}
	}

	return stepContext, err
}
