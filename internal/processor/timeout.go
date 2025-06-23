package processor

import (
	"context"
	"time"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithTimeout() ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Timeout.Duration == 0 {
			return nil
		}

		return &Timeout{
			timeout: spec.Timeout.Duration,
		}
	}
}

type Timeout struct {
	timeout time.Duration
}

func (s *Timeout) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		var cancel context.CancelFunc
		ctx.Context, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
		return next(ctx)
	}, nil
}
