package processor

import (
	"fmt"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithAncestors() ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		return &ancestors{
			taskName: spec.Name,
		}
	}
}

type ancestors struct {
	taskName string
}

type AncestorsContext struct {
	Refs []string
}

type ancestorsContext struct{}

func (s *ancestors) Bootstrap(pipelineCtx Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		deps := pipelineCtx.TaskDependencies(s.taskName)
		for _, dep := range deps {
			uniqueDep := dep
			if ctx.namespace != "" {
				uniqueDep = fmt.Sprintf("%s-%s", ctx.namespace, dep)
			}
			ctx.Ancestors.Refs = append(ctx.Ancestors.Refs, uniqueDep)
		}

		if ancestors, ok := ctx.Value(ancestorsContext{}).(string); ok {
			ctx.Ancestors.Refs = append(ctx.Ancestors.Refs, ancestors)
		}

		return next(ctx)
	}, nil
}
