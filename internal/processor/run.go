package processor

import (
	"context"
	"fmt"
	"io"

	"github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/internal/utils"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithRun(tee bool, defaultPullPolicy runtime.PullImagePolicy, driver runtime.Interface, outputFactory OutputFactory, stdin io.Reader, stdout, stderr io.Writer, teardown chan Teardown) ProcessorBuilder {
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

type Run struct {
	stepName          string
	step              v1beta1.RunStep
	driver            runtime.Interface
	defaultPullPolicy runtime.PullImagePolicy
	teardown          chan Teardown
}

func (s *Run) Substitute() []*Substitute {
	var vals []*Substitute

	vals = append(vals, &Substitute{
		v: s.step.Image,
		f: func(v interface{}) {
			s.step.Image = v.(string)
		},
	}, &Substitute{
		v: s.step.Args,
		f: func(v interface{}) {
			s.step.Args = v.([]string)
		},
	}, &Substitute{
		v: s.step.Command,
		f: func(v interface{}) {
			s.step.Command = v.([]string)
		},
	}, &Substitute{
		v: s.step.PWD,
		f: func(v interface{}) {
			s.step.PWD = v.(string)
		},
	})

	return vals
}

func (s *Run) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		pod := &runtime.Pod{
			Name: fmt.Sprintf("%s-%s-%s", PrefixName(stepContext.NamePrefix, s.stepName), pipeline.ID(), utils.RandString(5)),
			Spec: runtime.PodSpec{},
		}
		container := runtime.ContainerSpec{
			Name:            s.stepName,
			Stdin:           stepContext.Stdin != nil,
			TTY:             s.step.TTY,
			Image:           s.step.Image,
			ImagePullPolicy: s.defaultPullPolicy,
			Command:         s.step.Command,
			Args:            s.step.Args,
			Env:             envSlice(stepContext.Envs),
			PWD:             s.step.PWD,
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
