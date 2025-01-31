package processor

import (
	"context"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithAnd() ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.And == nil {
			return nil
		}

		return &And{
			refs: refSlice(spec.And.Refs),
		}
	}
}

type And struct {
	refs []string
}

func (s *And) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	steps, err := filterSteps(s.refs, pipeline)
	if err != nil {
		return nil, err
	}

	var stepEntrypoints []Next
	for _, step := range steps {
		next, err := step.Entrypoint()

		if err != nil {
			return next, err
		}

		stepEntrypoints = append(stepEntrypoints, next)
	}

	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		for _, next := range stepEntrypoints {
			var err error
			stepContext, err = next(ctx, stepContext)
			if AbortOnError(err) {
				return stepContext, err
			}
		}

		return next(ctx, stepContext)
	}, nil
}
