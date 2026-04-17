package processor

import (
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
			outCtx, err := next(parentCtx)
			outCtx.InputVars = ctx.InputVars
			ctx.Merge(outCtx)

			if err != nil {
				return ctx, err
			}
		}

		return next(ctx)
	}, nil
}
