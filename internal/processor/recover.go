package processor

import (
	"fmt"
	"runtime/debug"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithRecover() ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		return &Recover{
			stepName: spec.Name,
		}
	}
}

type Recover struct {
	stepName string
}

func (s *Recover) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (out TaskContext, err error) {
		out = ctx
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("panic occurred `%s`: %#v\n trace:\n%s", s.stepName, r, debug.Stack())
			}
		}()

		out, err = next(ctx)
		return
	}, nil
}
