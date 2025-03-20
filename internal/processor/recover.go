package processor

import (
	"context"
	"fmt"
	"runtime/debug"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithRecover() ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		return &Recover{
			stepName: spec.Name,
		}
	}
}

type Recover struct {
	stepName string
}

func (s *Recover) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx context.Context, stepContext StepContext) (out StepContext, err error) {
		out = stepContext
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("panic in step `%s`: %#v\n trace:\n%s", s.stepName, r, debug.Stack())
			}
		}()

		out, err = next(ctx, stepContext)
		return
	}, nil
}
