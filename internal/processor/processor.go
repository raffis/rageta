package processor

import (
	"context"
	"time"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

type PipelineBuilder interface {
	Build(pipeline v1beta1.Pipeline, entrypoint string, inputs map[string]v1beta1.ParamValue, stepCtx TaskContext) (Executable, error)
}

type Executable func() (TaskContext, map[string]v1beta1.ParamValue, error)

type Pipeline interface {
	Task(name string) (Task, error)
	TasksByLabels(labels map[string]string) []Task
	DependantTasks(name string) []Task
	TaskDependencies(name string) []string
	Entrypoint(name string) (Next, error)
	EntrypointName() (string, error)
	Name() string
	ID() string
}

type Next func(ctx TaskContext) (TaskContext, error)

type Bootstraper interface {
	Bootstrap(pipeline Pipeline, next Next) (Next, error)
}

type Task interface {
	Processors() []Bootstraper
	Entrypoint() (Next, error)
	Name() string
}

type Teardown func(ctx context.Context, timeout time.Duration) error

type result struct {
	ctx TaskContext
	err error
}

type ProcessorBuilder func(spec *v1beta1.Task) Bootstraper

func Builder(spec *v1beta1.Task, builders ...ProcessorBuilder) []Bootstraper {
	var result []Bootstraper
	for _, builder := range builders {
		processor := builder(spec)
		if processor != nil {
			result = append(result, processor)
		}
	}

	return result
}
