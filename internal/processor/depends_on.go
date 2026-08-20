package processor

import (
	"context"
	"errors"

	"github.com/moby/buildkit/client/llb"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithDependsOn() ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		return &DependsOn{
			deps:     spec.DependsOn,
			taskName: spec.Name,
		}
	}
}

type DependsOn struct {
	deps     []v1beta1.TaskDependency
	taskName string
}

func (s *DependsOn) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		ctx, err := next(ctx)

		if err != nil && AbortOnError(err) {
			return ctx, err
		}

		ctx.Tasks[s.taskName] = &ctx
		var children []Task

		for _, task := range pipeline.ChildTasks(s.taskName) {
			if _, started := ctx.Tasks[task.Name()]; started {
				continue
			}

			// Only launch the child once all of its dependencies have
			// completed, not just this one.
			if !task.Ready(ctx) {
				continue
			}

			mergeDependencyResults(pipeline, ctx, task.Name())
			children = append(children, task)
		}

		childCtx, childErr := launchTasks(ctx, children)
		if err != nil {
			if childErr != nil {
				return childCtx, errors.Join(err, childErr)
			}
			return childCtx, err
		}

		return childCtx, childErr
	}, nil
}

// mergeDependencyResults pulls the results of taskName's dependencies into
// ctx before it is launched. Each dependency finished on its own branch of
// ctx, so its result (in particular ctx.Tasks[depName], needed e.g. for
// service binding resolution) is only visible there.
func mergeDependencyResults(pipeline Pipeline, ctx TaskContext, taskName string) {
	for _, depName := range pipeline.TaskDependencies(taskName) {
		if _, ok := ctx.Tasks[depName]; ok {
			continue
		}

		dep, err := pipeline.Task(depName)
		if err != nil {
			continue
		}

		claim, owner := dep.Claim(ctx)
		if owner {
			claim.Release(ctx.DeepCopy(), nil)
			continue
		}

		if depCtx, depErr := claim.Wait(); depErr == nil || !AbortOnError(depErr) {
			ctx.Merge(depCtx)
		}
	}
}

// launchTasks claims and runs tasks against ctx, waiting for all of them to
// finish before returning the merged context.
func launchTasks(ctx TaskContext, tasks []Task) (TaskContext, error) {
	if len(tasks) == 0 {
		return ctx, nil
	}

	results := make(chan result)
	var errs []error

	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var launched int
	for _, task := range tasks {
		claim, owner := task.Claim(ctx)
		launched++

		if !owner {
			go func(claim TaskClaim) {
				t, err := claim.Wait()

				copyCtx := t.DeepCopy()
				copyCtx.Build.State = llb.Scratch()
				copyCtx.Context = cancelCtx

				results <- result{copyCtx, err}
			}(claim)
			continue
		}

		next, err := task.Entrypoint()
		if err != nil {
			claim.Release(ctx.DeepCopy(), err)
			return ctx, err
		}

		copyCtx := ctx.DeepCopy()
		copyCtx.Context = cancelCtx

		go func(claim TaskClaim) {
			t, err := next(copyCtx)
			claim.Release(t, err)
			results <- result{t, err}
		}(claim)
	}

	if launched == 0 {
		return ctx, nil
	}

	var done int
WAIT:
	for res := range results {
		done++

		res.ctx.InputVars = ctx.InputVars
		ctx.Merge(res.ctx)

		switch {
		case cancelCtx.Err() == context.Canceled && len(errs) > 0:
		case res.err != nil && AbortOnError(res.err):
			errs = append(errs, res.err)
		default:
		}

		if done == launched {
			break WAIT
		}
	}

	if len(errs) > 0 {
		return ctx, errors.Join(errs...)
	}

	return ctx, nil
}
