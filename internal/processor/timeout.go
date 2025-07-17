package processor

import (
	"context"
	"errors"
	"time"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithTimeout() ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Timeout.Duration == 0 {
			return nil
		}

		return &Timeout{
			timer: time.NewTimer(spec.Timeout.Duration),
		}
	}
}

var ErrTimeout = errors.New("operation timed out")

type Timeout struct {
	timer *time.Timer
}

func (s *Timeout) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		copyCtx := ctx

		var cancel context.CancelFunc
		copyCtx.Context, cancel = context.WithCancel(ctx)
		done := make(chan result)

		defer cancel()

		go func() {
			ctx, err := next(copyCtx)
			done <- result{ctx, err}
		}()

		select {
		case <-ctx.Done():
			return ctx, ctx.Err()
		case <-s.timer.C:
			return ctx, ErrTimeout
		case result := <-done:
			result.ctx.Context = ctx.Context
			return result.ctx, result.err
		}
	}, nil
}
