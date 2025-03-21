package processor

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/internal/utils"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithRun(tee bool, defaultPullPolicy runtime.PullImagePolicy, driver runtime.Interface, outputFactory OutputFactory, stdin io.Reader, stdout, stderr io.Writer, teardown chan Teardown) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Run == nil {
			return nil
		}
		fmt.Printf("INIUT \n\n%#v\n", spec.Run.Script)

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

func (s *Run) Substitute() []interface{} {
	return []interface{}{
		&s.step.Image,
		s.step.Args,
		s.step.Command,
		&s.step.Script,
		&s.step.WorkDir,
	}
}

func (s *Run) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		pod := &runtime.Pod{
			Name: fmt.Sprintf("%s-%s-%s", PrefixName(stepContext.NamePrefix, s.stepName), pipeline.ID(), utils.RandString(5)),
			Spec: runtime.PodSpec{},
		}

		command, args := s.commandArgs()

		container := runtime.ContainerSpec{
			Name:            s.stepName,
			Stdin:           stepContext.Stdin != nil,
			TTY:             s.step.TTY,
			Image:           s.step.Image,
			ImagePullPolicy: s.defaultPullPolicy,
			Command:         command,
			Args:            args,
			Env:             envSlice(stepContext.Envs),
			PWD:             s.step.WorkDir,
			RestartPolicy:   runtime.RestartPolicy(s.step.RestartPolicy),
		}

		pod.Spec.Containers = []runtime.ContainerSpec{container}
		stepContext, err := s.exec(ctx, stepContext, pod, stepContext.Stdin)

		if err != nil {
			return stepContext, fmt.Errorf("container %s failed: %w", pod.Name, err)
		}

		return next(ctx, stepContext)
	}, nil
}

func (s *Run) commandArgs() ([]string, []string) {
	fmt.Printf("SSSSSSSSSSS \n\n%#v\n", s.step.Script)

	script := strings.TrimSpace(s.step.Script)
	args := s.step.Args

	if script == "" {
		return s.step.Command, s.step.Args
	}

	hasShebang := strings.HasPrefix(script, "#!")

	if !hasShebang {
		script = defaultScriptHeader + script
	}

	header := strings.Split(script, "\n")[0]
	shebang := strings.Split(header, "#!")
	command := []string{shebang[1]}

	fmt.Printf("SSSSSSSSSSS \n\n%#v\n", script)

	return command, append(args, "-c", script)
}

func envSlice(env map[string]string) []string {
	var envs []string
	for k, v := range env {
		envs = append(envs, fmt.Sprintf("%s=%s", k, v))
	}

	return envs
}

func (s *Run) exec(ctx context.Context, stepContext StepContext, pod *runtime.Pod, stdin io.Reader) (StepContext, error) {
	await, err := s.driver.CreatePod(ctx, pod, stdin, stepContext.Stdout, stepContext.Stderr)
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
				fmt.Printf("\nWAIT ERR %#v\n\n", err)
				done <- err
			}
			fmt.Printf("\nWAIT done\n\n", err)

			done <- nil
		}()

		s.teardown <- func(ctx context.Context) error {
			fmt.Printf("\n\nTEARDOWN\n\n")
			return <-done
		}

	} else {
		if err := await.Wait(); err != nil {
			return stepContext, err
		}
	}

	return stepContext, err
}
