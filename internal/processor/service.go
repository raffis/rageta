package processor

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"time"

	"github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/internal/substitute"
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
		container := &runtime.Container{
			Name: s.stepName,
		}

		if err := substitute.Substitute(ctx.ToV1Beta1(), svc.Guid, svc.Uid); err != nil {
			return ctx, err
		}

		envs := make(map[string]string)
		maps.Copy(envs, ctx.EnvVars.Envs)
		maps.Copy(envs, ctx.SecretVars.Secrets)

		spec := runtime.ContainerSpec{
			Image:           s.image,
			ImagePullPolicy: s.defaultPullPolicy,
			Command:         svc.Command,
			Args:            svc.Args,
			Env:             envs,
			PWD:             s.workdir,
		}

		if svc.Guid != nil {
			guid := svc.Guid.IntValue()
			spec.Guid = &guid
		}

		if svc.Uid != nil {
			uid := svc.Uid.IntValue()
			spec.Uid = &uid
		}

		subst := []any{
			&spec.Image,
			spec.Args,
			spec.Command,
			&spec.PWD,
		}

		if err := substitute.Substitute(ctx.ToV1Beta1(), subst...); err != nil {
			return ctx, err
		}

		container.Spec = spec
		_, _ = ctx.Display.Dev.Write([]byte(fmt.Sprintf("starting %s", spec.Image) + "\n"))
		ctx, err := s.exec(ctx, container)

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

func (s *Service) exec(ctx TaskContext, container *runtime.Container) (TaskContext, error) {
	await, err := s.driver.Create(ctx, container, nil, ctx.Display.Stdout, ctx.Display.Stderr)
	if err != nil {
		return ctx, err
	}

	ctx.Services.Status[container.Name] = container.Status

	done := make(chan error)
	go func() {
		if err := await.Wait(ctx); err != nil {
			done <- err
		}

		done <- nil
	}()

	s.teardown <- func(teardownCtx context.Context, timeout time.Duration) error {
		if containerStatus, ok := ctx.Services.Status[s.stepName]; ok {
			err := s.driver.Delete(teardownCtx, &runtime.Container{
				Status: containerStatus,
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
	return fmt.Sprintf("service failed: %s", e.parent.Error())
}

func (e *serviceError) Unwrap() error {
	return e.parent
}

func (e *serviceError) ExitCode() int {
	return e.exitCode
}
