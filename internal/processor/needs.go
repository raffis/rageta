package processor

import (
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
			parentCtx.Dir = ctx.Dir
			parentCtx.Template = ctx.Template.DeepCopy()

			outCtx, err := next(parentCtx)
			outCtx.Inputs = ctx.Inputs
			ctx.Merge(outCtx)

			if err != nil {
				return ctx, err
			}
		}

		return next(ctx)
	}, nil
}
