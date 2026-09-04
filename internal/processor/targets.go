package processor

import (
	"context"
	"errors"
	"maps"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithTargets() ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		if spec.Targets == nil {
			return nil
		}

		return &Targets{
			taskName: spec.Name,
			refs:     *spec.Targets,
		}
	}
}

type Targets struct {
	taskName string
	refs     []v1beta1.LocalReference
}

func (s *Targets) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		results := make(chan result)
		var errs []error

		cancelCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		for _, ref := range s.refs {
			task, err := pipeline.Task(ref.Name)
			if err != nil {
				return ctx, err
			}

			next, err := task.Entrypoint()
			if err != nil {
				return ctx, err
			}

			copyCtx := ctx.DeepCopy().WithNamespace(s.taskName)
			copyCtx.Context = cancelCtx
			copyCtx.Context = context.WithValue(cancelCtx, ancestorsContext{}, ctx.UniqueName())

			go func() {
				copyCtx, err := next(copyCtx)
				results <- result{copyCtx, err}
			}()
		}

		var (
			done      int
			allCached bool = true
		)
	WAIT:
		for res := range results {
			done++

			/*if child, ok := res.ctx.Tasks[s.taskName]; ok {
				ctx.TaskGroups[s.taskName] = append(ctx.TaskGroups[s.taskName], child)
			}

			for name, instances := range res.ctx.TaskGroups {
				ctx.TaskGroups[name] = append(ctx.TaskGroups[name], instances...)
			}*/

			maps.Copy(ctx.Tasks, res.ctx.Tasks)

			if !res.ctx.Build.Cached {
				allCached = false
			}

			switch {
			case cancelCtx.Err() == context.Canceled && len(errs) > 0:
			case res.err != nil && AbortOnError(res.err):
				errs = append(errs, res.err)

				//if s.failFast {
				//	cancel()
				//}
			default:
			}

			if done == len(s.refs) {
				break WAIT
			}
		}

		ctx.Build.Cached = allCached
		return ctx, errors.Join(errs...)
	}, nil

}
