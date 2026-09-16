package processor

import (
	"github.com/raffis/rageta/internal/secrets"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithSteps(store secrets.Interface, noCache bool) ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		if spec.Steps == nil {
			return nil
		}

		return &Steps{
			steps:    spec.Steps,
			taskName: spec.Name,
			store:    store,
			noCache:  noCache,
		}
	}
}

type Steps struct {
	steps    []v1beta1.Step
	taskName string
	store    secrets.Interface
	noCache  bool
}

func (s *Steps) Bootstrap(_ Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		ctx, err = next(ctx)
		if err != nil {
			return ctx, err
		}

		return ctx, nil
	}, nil
}
