package processor

import (
	"context"
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
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		stepContext.StartedAt = time.Now()
		stepContext, nextErr := next(ctx, stepContext)
		stepContext.EndedAt = time.Now()

		if nextErr != nil {
			nextErr = fmt.Errorf("step %s failed: %w", s.stepName, nextErr)
			stepContext.Error = nextErr
		} else {
			stepContext.Error = nil
		}

		stepContext.Steps[s.stepName] = &stepContext
		return stepContext, nextErr
	}, nil
}
