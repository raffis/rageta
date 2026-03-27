package processor

import (
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithMaxConcurrent(pool chan struct{}) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Run == nil || pool == nil {
			return nil
		}

		return &MaxConcurrent{
			pool: pool,
		}
	}
}

type MaxConcurrent struct {
	pool chan struct{}
}

func (s *MaxConcurrent) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		s.pool <- struct{}{}
		defer func() {
			<-s.pool
		}()

		return next(ctx)
	}, nil
}
