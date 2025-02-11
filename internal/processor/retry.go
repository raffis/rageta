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

	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		var err error
		if err := retry.Do(ctx, backoff, func(ctx context.Context) error {
			stepContext, err = next(ctx, stepContext)
			if err != nil {
				return retry.RetryableError(err)
			}

			return nil
		}); err != nil {
			return stepContext, err
		}

		return stepContext, err
	}, nil
}
