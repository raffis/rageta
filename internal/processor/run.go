package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/internal/utils"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

type OutputCloser func(err error)
type OutputFactory func(name string, stdin io.Reader, stdout, stderr io.Writer) (io.Reader, io.Writer, io.Writer, OutputCloser)

func WithRun(tee bool, defaultPullPolicy runtime.PullImagePolicy, driver runtime.Interface, outputFactory OutputFactory, stdin io.Reader, stdout, stderr io.Writer) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Run == nil {
			return nil
		}

		return &Run{
			rawStep:           *spec.Run,
			stepName:          spec.Name,
			tee:               tee,
			outputFactory:     outputFactory,
			driver:            driver,
			stdin:             stdin,
			stdout:            stdout,
			stderr:            stderr,
			defaultPullPolicy: defaultPullPolicy,
		}
	}
}

type Run struct {
	stepName          string
	stdin             io.Reader
	stdout            io.Writer
	stderr            io.Writer
	rawStep           v1beta1.RunStep
	step              v1beta1.RunStep
	tee               bool
	outputFactory     OutputFactory
	driver            runtime.Interface
	defaultPullPolicy runtime.PullImagePolicy
}

func (s *Run) UnmarshalJSON(b []byte) error {
	return json.Unmarshal(b, &s.step)
}

func (s *Run) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.rawStep)
}

func (s *Run) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		pod := &runtime.Pod{
			Name: fmt.Sprintf("%s-%s-%s", PrefixName(s.stepName, stepContext.NamePrefix), pipeline.ID(), utils.RandString(5)),
			Spec: runtime.PodSpec{},
		}

		_, stdout, stderr, close := s.outputFactory(PrefixName(s.stepName, stepContext.NamePrefix), s.stdin, s.stdout, s.stderr)
		var stdin io.Reader

		if stepContext.Stdout.Len() > 0 {
			if s.tee {
				stepContext.Stdout.Add(stdout)
			}
		} else {
			stepContext.Stdout.Add(stdout)
		}

		if stepContext.Stderr.Len() > 0 {
			if s.tee {
				stepContext.Stderr.Add(stderr)
			}
		} else {
			stepContext.Stderr.Add(stderr)
		}

		if stepContext.Stdin != nil {
			stdin = stepContext.Stdin
		}

		container := runtime.ContainerSpec{
			Name:            s.stepName,
			Stdin:           stdin != nil,
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
		stepContext, execErr := s.exec(ctx, stepContext, pod, stdin)
		defer close(execErr)

		stepContext.Stderr.Remove(stderr)
		stepContext.Stdout.Remove(stdout)

		if execErr != nil {
			return stepContext, fmt.Errorf("container %s failed: %w", pod.Name, execErr)
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

func (e *Run) exec(ctx context.Context, stepContext StepContext, pod *runtime.Pod, stdin io.Reader) (StepContext, error) {
	await, err := e.driver.CreatePod(ctx, pod, stdin, stepContext.Stdout, stepContext.Stderr)
	if err != nil {
		return stepContext, err
	}

	for _, v := range pod.Status.Containers {
		stepContext.Containers[v.Name] = v
	}

	if e.step.Await == v1beta1.AwaitStatusReady {
		go func() {
			if err := await.Wait(); err != nil {
				panic(err)
			}

		}()
	} else {
		if err := await.Wait(); err != nil {
			return stepContext, err
		}
	}

	return stepContext, err
}
