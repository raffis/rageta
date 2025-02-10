package processor

import (
	"context"
	"fmt"
	"os"
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
		outputTmp, err := os.CreateTemp(stepContext.TmpDir(), "output")
		if err != nil {
			return stepContext, err
		}

		defer outputTmp.Close()
		defer os.Remove(outputTmp.Name())

		stepContext.Output = outputTmp.Name()
		stepContext.Steps[s.stepName] = &StepResult{
			StartedAt: start,
		}

		stepContext, nextErr := next(ctx, stepContext)
		endedAt := time.Now()
		outputTmp.Sync()
		outputs, err := parseVars(outputTmp)
		if err != nil {
			return stepContext, err
		}

		stepContext.Steps[s.stepName].Outputs = outputs
		stepContext.Steps[s.stepName].EndedAt = endedAt
		stepContext.Steps[s.stepName].Error = nextErr

		if nextErr != nil {
			nextErr = fmt.Errorf("step %s failed: %w", s.stepName, nextErr)
		}

		return stepContext, nextErr

	}, nil
}
