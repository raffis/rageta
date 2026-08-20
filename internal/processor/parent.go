package processor

import (
	"fmt"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithParent() ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		return &parent{
			taskName: spec.Name,
		}
	}
}

type parent struct {
	taskName string
}

type ParentContext struct {
	Refs []string
}

type parentContext struct{}

func (s *parent) Bootstrap(pipelineCtx Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		deps := pipelineCtx.TaskDependencies(s.taskName)
		for _, dep := range deps {
			uniqueDep := dep
			if ctx.namespace != "" {
				uniqueDep = fmt.Sprintf("%s-%s", ctx.namespace, dep)
			}
			ctx.Parent.Refs = append(ctx.Parent.Refs, uniqueDep)
		}

		if parent, ok := ctx.Value(parentContext{}).(string); ok {
			ctx.Parent.Refs = append(ctx.Parent.Refs, parent)
		}

		return next(ctx)
	}, nil
}
