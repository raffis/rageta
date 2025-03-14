package processor

import (
	"context"
	"errors"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithConcurrent() ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Concurrent == nil {
			return nil
		}

		return &Concurrent{
			refs:     refSlice(spec.Concurrent.Refs),
			failFast: spec.Concurrent.FailFast,
		}
	}
}

type Concurrent struct {
	failFast bool
	refs     []string
}

type concurrentResult struct {
	stepContext StepContext
	err         error
}

func (s *Concurrent) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
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
		results := make(chan concurrentResult)
		var errs []error

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		for _, next := range stepEntrypoints {
			next := next
			stepContext := stepContext.DeepCopy()
			go func() {
				t, err := next(ctx, stepContext)
				results <- concurrentResult{t, err}
			}()
		}

		var done int
	WAIT:
		for res := range results {
			done++
			stepContext = stepContext.Merge(res.stepContext)
			if res.err != nil && AbortOnError(res.err) {
				errs = append(errs, res.err)

				if s.failFast {
					cancel()
				}
			}

			if done == len(steps) {
				break WAIT
			}
		}

		if len(errs) > 0 {
			return stepContext, errors.Join(errs...)
		}

		return next(ctx, stepContext)
	}, nil
}
