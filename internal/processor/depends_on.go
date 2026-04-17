package processor

import (
	"context"
	"errors"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithDependsOn() ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if len(spec.DependsOn) == 0 {
			return nil
		}

		return &Needs{
			refs: refSlice(spec.DependsOn),
		}
	}
}

type Needs struct {
	refs []string
}

func (s *Needs) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		results := make(chan result)
		var errs []error

		cancelCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		for _, needsStepName := range s.refs {
			stepExecuted := false
			for stepName := range ctx.Steps {
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
				return ctx, err
			}

			next, err := step.Entrypoint()
			if err != nil {
				return ctx, err
			}

			parentCtx := NewContext()
			parentCtx.ContextDir = ctx.ContextDir
			parentCtx.Context = ctx.Context

			go func() {
				t, err := next(parentCtx)
				results <- result{t, err}
			}()
		}

		var done int
	WAIT:
		for res := range results {
			done++

			res.ctx.InputVars = ctx.InputVars
			ctx.Merge(res.ctx)

			switch {
			case cancelCtx.Err() == context.Canceled && len(errs) > 0:
			case res.err != nil && AbortOnError(res.err):
				errs = append(errs, res.err)

				//if s.failFast {
				//	cancel()
				//}
			default:
			}

			if done == len(s.refs) {
				break WAIT
			}
		}

		if len(errs) > 0 {
			return ctx, errors.Join(errs...)
		}

		return next(ctx)
	}, nil
}
