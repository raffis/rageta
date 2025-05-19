package processor

import (
	"errors"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithSkipDone(skipDone bool) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if !skipDone {
			return nil
		}

		return &SkipDone{
			stepName: spec.Name,
		}
	}
}

type SkipDone struct {
	stepName string
}

var ErrSkipDone = errors.New("skip step marked as successful")

func (s *SkipDone) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		skipExecution := false
		for stepName, result := range ctx.Steps {
			if stepName == s.stepName && result.EndedAt.IsZero() && result.Error != nil {
				skipExecution = true
				break
			}
		}

		if !skipExecution {
			return next(ctx)
		}

		return ctx, ErrSkipDone
	}, nil
}
