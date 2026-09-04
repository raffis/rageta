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
		// Always recompute this task's ancestor refs from scratch rather than
		// appending onto whatever ctx.Ancestors.Refs already carried in. A
		// DeepCopy (e.g. Targets/Inherit/Matrix spawning a child's starting
		// ctx from the launching task's own ctx) copies the launcher's own
		// Ancestors.Refs forward; appending onto that here would leave a
		// nested task's ancestors polluted with entries from further up the
		// chain instead of just its own direct dependencies/launcher, which
		// can make the tree UI place it under the wrong (or no) ancestor.
		ctx.Ancestors.Refs = nil

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
