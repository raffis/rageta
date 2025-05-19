package processor

import (
	"fmt"
	"time"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithResult() ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		return &Result{
			stepName: spec.Name,
		}
	}
}

type Result struct {
	stepName string
}

func (s *Result) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		ctx.StartedAt = time.Now()
		ctx, nextErr := next(ctx)
		ctx.EndedAt = time.Now()

		if nextErr != nil {
			nextErr = fmt.Errorf("step %s failed: %w", s.stepName, nextErr)
			ctx.Error = nextErr
		} else {
			ctx.Error = nil
		}

		ctx.Steps[s.stepName] = &ctx
		return ctx, nextErr
	}, nil
}
