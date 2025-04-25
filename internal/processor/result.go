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
		start := time.Now()
		stepContext.Steps[s.stepName] = &StepResult{
			StartedAt: start,
			Outputs:   make(map[string]v1beta1.ParamValue),
		}

		stepContext, nextErr := next(ctx, stepContext)
		endedAt := time.Now()
		stepContext.Steps[s.stepName].EndedAt = endedAt
		stepContext.Steps[s.stepName].Error = nextErr

		if nextErr != nil {
			nextErr = fmt.Errorf("step %s failed: %w", s.stepName, nextErr)
		}

		return stepContext, nextErr

	}, nil
}
