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

type stepError struct {
	parent         error
	stepName       string
	uniqueStepName string
}

func (e *stepError) Error() string {
	return fmt.Sprintf("step %s failed: %s", e.stepName, e.parent.Error())
}

func (e *stepError) Unwrap() error {
	return e.parent
}

func (e *stepError) StepName() string {
	return e.stepName
}

func (s *Result) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		ctx.StartedAt = time.Now()
		ctx, nextErr := next(ctx)
		ctx.EndedAt = time.Now()

		if nextErr != nil {
			ctx.Error = &stepError{
				parent:         nextErr,
				stepName:       s.stepName,
				uniqueStepName: SuffixName(s.stepName, ctx.NamePrefix),
			}
		} else {
			ctx.Error = nil
		}

		ctx.Steps[s.stepName] = &ctx
		return ctx, nextErr
	}, nil
}
