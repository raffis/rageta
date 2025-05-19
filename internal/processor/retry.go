package processor

import (
	"context"
	"time"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"github.com/sethvargo/go-retry"
)

func WithRetry() ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Retry == nil {
			return nil
		}

		return &Retry{
			max:         uint64(spec.Retry.MaxRetries),
			exponential: spec.Retry.Exponential.Duration,
			constant:    spec.Retry.Constant.Duration,
		}
	}
}

type Retry struct {
	max         uint64
	exponential time.Duration
	constant    time.Duration
}

func (s *Retry) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	var backoff retry.Backoff
	switch {
	case s.exponential != 0:
		backoff = retry.NewExponential(s.exponential)
	case s.constant != 0:
		backoff = retry.NewConstant(s.constant)
	}

	if s.max != 0 {
		backoff = retry.WithMaxRetries(s.max, backoff)
	}

	return func(stepCtx StepContext) (StepContext, error) {
		var err error
		if err := retry.Do(stepCtx, backoff, func(ctx context.Context) error {
			stepCtx.Context = ctx
			stepCtx, err = next(stepCtx)
			if err != nil {
				return retry.RetryableError(err)
			}

			return nil
		}); err != nil {
			return stepCtx, err
		}

		return stepCtx, err
	}, nil
}
