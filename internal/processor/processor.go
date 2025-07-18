package processor

import (
	"context"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

type PipelineBuilder interface {
	Build(pipeline v1beta1.Pipeline, entrypoint string, inputs map[string]v1beta1.ParamValue, stepCtx StepContext) (Executable, error)
}

type Executable func() (StepContext, map[string]v1beta1.ParamValue, error)

type Pipeline interface {
	Step(name string) (Step, error)
	Entrypoint(name string) (Next, error)
	EntrypointName() (string, error)
	Name() string
	ID() string
}

type Next func(ctx StepContext) (StepContext, error)

type Bootstraper interface {
	Bootstrap(pipeline Pipeline, next Next) (Next, error)
}

type Step interface {
	Processors() []Bootstraper
	Entrypoint() (Next, error)
}

type Teardown func(ctx context.Context) error

type result struct {
	ctx StepContext
	err error
}
