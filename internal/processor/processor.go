package processor

import (
	"context"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

type PipelineBuilder interface {
	Build(pipeline v1beta1.Pipeline, entrypoint string, inputs map[string]v1beta1.ParamValue, template *v1beta1.Template) (Executable, error)
}

type Executable func(ctx context.Context) (StepContext, map[string]v1beta1.ParamValue, error)

type Pipeline interface {
	Step(name string) (Step, error)
	Entrypoint(name string) (Next, error)
	EntrypointName() (string, error)
	Name() string
	ID() string
}

type Next func(ctx context.Context, stepContext StepContext) (StepContext, error)

type Bootstraper interface {
	Bootstrap(pipeline Pipeline, next Next) (Next, error)
}

type Step interface {
	Processors() []Bootstraper
	Entrypoint() (Next, error)
}

type Teardown func(ctx context.Context) error
