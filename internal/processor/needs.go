package processor

import (
	"context"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithNeeds() ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Needs == nil {
			return nil
		}

		return &Needs{
			refs: refSlice(spec.Needs),
		}
	}
}

type Needs struct {
	refs []string
}

func (s *Needs) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		for _, needsStepName := range s.refs {
			stepExecuted := false
			for stepName := range stepContext.Steps {
				if stepName == needsStepName {
					stepExecuted = true
					break
				}
			}

			if stepExecuted {
				continue
			}

			step, err := pipeline.Step(needsStepName)
			if err != nil {
				return stepContext, err
			}

			next, err := step.Entrypoint()
			if err != nil {
				return stepContext, err
			}

			stepContext, err = next(ctx, stepContext)

			if err != nil {
				return stepContext, err
			}
		}

		return next(ctx, stepContext)
	}, nil
}
