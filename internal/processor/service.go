package processor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"strings"
	"time"

	"github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/internal/utils"
	"github.com/raffis/rageta/internal/xio"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithService(defaultPullPolicy runtime.PullImagePolicy, driver runtime.Interface, teardown chan Teardown) ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		if spec.Service == nil {
			return nil
		}

		return &Service{
			service:           *spec.Service,
			image:             spec.Image,
			workdir:           spec.WorkingDir,
			stepName:          spec.Name,
			driver:            driver,
			defaultPullPolicy: defaultPullPolicy,
			teardown:          teardown,
		}
	}
}

type Service struct {
	image             string
	workdir           string
	stepName          string
	service           v1beta1.ServiceTask
	driver            runtime.Interface
	defaultPullPolicy runtime.PullImagePolicy
	teardown          chan Teardown
}

type ServiceContext struct {
	Status map[string]runtime.ContainerStatus
}

func newServiceContext() ServiceContext {
	return ServiceContext{
		Status: make(map[string]runtime.ContainerStatus),
	}
}

func (s *Service) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		svc := s.service.DeepCopy()
		pod := &runtime.Pod{
			Name: fmt.Sprintf("rageta-%s-%s-%s", pipeline.ID(), ctx.UniqueID(), utils.RandString(5)),
			Spec: runtime.PodSpec{},
		}

		if err := substitute.Substitute(ctx.ToV1Beta1(), svc.Guid, svc.Uid); err != nil {
			return ctx, err
		}

		envs := make(map[string]string)
		maps.Copy(envs, ctx.EnvVars.Envs)
		maps.Copy(envs, ctx.SecretVars.Secrets)

		container := runtime.ContainerSpec{
			Name:            s.stepName,
			Image:           s.image,
			ImagePullPolicy: s.defaultPullPolicy,
			Command:         svc.Command,
			Args:            svc.Args,
			Env:             envs,
			PWD:             s.workdir,
		}

		if svc.Guid != nil {
			guid := svc.Guid.IntValue()
			container.Guid = &guid
		}

		if svc.Uid != nil {
			uid := svc.Uid.IntValue()
			container.Uid = &uid
		}

		subst := []any{
			&container.Image,
			container.Args,
			container.Command,
			&container.PWD,
		}

		if err := substitute.Substitute(ctx.ToV1Beta1(), subst...); err != nil {
			return ctx, err
		}

		pod.Spec.Containers = []runtime.ContainerSpec{container}
		_, _ = ctx.Events.Dev.Write([]byte(fmt.Sprintf("🐋 starting %s", container.Image) + "\n"))
		ctx, err := s.exec(ctx, pod)

		if err != nil {
			var exitCode int
			var runtimeErr ExitCode
			if errors.As(err, &runtimeErr) {
				exitCode = runtimeErr.ExitCode()
			}

			return ctx, &serviceError{
				exitCode: exitCode,
				parent:   err,
			}
		}

		return next(ctx)
	}, nil
}

func (s *Service) exec(ctx TaskContext, pod *runtime.Pod) (TaskContext, error) {
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
		ctx.Services.Status[v.Name] = v
	}

	done := make(chan error)
	go func() {
		if err := await.Wait(ctx); err != nil {
			done <- err
		}

		done <- nil
	}()

	s.teardown <- func(teardownCtx context.Context, timeout time.Duration) error {
		if containerStatus, ok := ctx.Services.Status[s.stepName]; ok {
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

type serviceError struct {
	exitCode int
	parent   error
}

func (e *serviceError) Error() string {
	return fmt.Sprintf("script failed: %s", e.parent.Error())
}

func (e *serviceError) Unwrap() error {
	return e.parent
}

func (e *serviceError) ExitCode() int {
	return e.exitCode
}
