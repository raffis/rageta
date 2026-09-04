package processor

import (
	"context"
	"errors"
	"sync"

	"github.com/moby/buildkit/client/llb"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

// selfReleaseKey holds the release callback launchTasks stashes on a task's
// context before running it as claim owner (see launchTasks). A processor
// that knows its own work is done before the rest of the chain returns (e.g.
// DependsOn, once it has recorded ctx.Tasks[name], or Matrix, once every
// instance has finished) can call releaseSelf to release the claim early,
// rather than only once the whole chain — including eagerly launched
// children/dependents — returns.
//
// This matters because ensureDependencies claims and runs a task's
// dependencies directly (not just merges already-finished results), and
// DependsOn/Matrix also eagerly launch a completed task's children before
// returning. Diamond shapes (B depends on A, and C depends on both A and B)
// used to deadlock: C's ensureDependencies would launch A and B as
// concurrent siblings, B would claim-and-wait on A via its own
// ensureDependencies, while A — still holding its own claim open while it
// eagerly pushed B as a ready child — would block claiming B, which B's
// goroutine already owned. Releasing a task's claim as soon as its own work
// is recorded (instead of after its pushed children finish too) breaks that
// cycle: waiters only need the task's own result, not its descendants'.
type selfReleaseKey struct{}

// releaseSelf calls the release callback stashed on ctx by launchTasks, if
// any. It's a no-op for contexts not produced by launchTasks (e.g. tests
// constructing a TaskContext directly).
func releaseSelf(ctx TaskContext, t TaskContext, err error) {
	if fn, ok := ctx.Value(selfReleaseKey{}).(func(TaskContext, error)); ok {
		fn(t, err)
	}
}

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
		// Run this task's own dependencies first. When the task is reached
		// via another task's downstream launch (below) its dependencies were
		// already ensured by the launching task, so this is a no-op there
		// (they're already in ctx.Tasks). But when the task is invoked
		// directly, e.g. as the CLI entrypoint (-t), nothing upstream has
		// run its dependencies yet, so it must do so itself here.
		ctx, depErr := ensureDependencies(pipeline, ctx, s.taskName)

		ctx, err := next(ctx)

		if depErr != nil {
			if err != nil {
				err = errors.Join(err, depErr)
			} else {
				err = depErr
			}
		}

		if err != nil && AbortOnError(err) {
			return ctx, err
		}

		ctx.Tasks[s.taskName] = &ctx

		// The task's own work is done and recorded; release its claim now
		// so anything waiting on it (a sibling that depends on it directly,
		// e.g. a dependent of this task's own dependency) can proceed
		// without waiting on the children this task is about to eagerly
		// launch below. See selfReleaseKey.
		releaseSelf(ctx, ctx, err)

		var children []Task
		var siblingDepErrs []error

		for _, task := range pipeline.ChildTasks(s.taskName) {
			if _, started := ctx.Tasks[task.Name()]; started {
				continue
			}

			// Only launch the child once all of its dependencies have
			// completed, not just this one.
			if !task.Ready(ctx) {
				continue
			}

			var siblingDepErr error
			ctx, siblingDepErr = ensureDependencies(pipeline, ctx, task.Name())
			if siblingDepErr != nil {
				siblingDepErrs = append(siblingDepErrs, siblingDepErr)
			}

			children = append(children, task)
		}

		childCtx, childErr := launchTasks(ctx, children)
		childErr = errors.Join(append(siblingDepErrs, childErr)...)

		if err != nil {
			if childErr != nil {
				return childCtx, errors.Join(err, childErr)
			}
			return childCtx, err
		}

		return childCtx, childErr
	}, nil
}

// ensureDependencies runs, or waits for, any of taskName's dependencies that
// haven't completed against ctx yet, merging their results in. A dependency
// already claimed (or completed) elsewhere is only waited on, never re-run;
// the actual work happens through the normal launchTasks/Claim machinery so
// a dependency that nobody else has started yet is genuinely executed rather
// than assumed to have finished.
func ensureDependencies(pipeline Pipeline, ctx TaskContext, taskName string) (TaskContext, error) {
	var deps []Task
	for _, depName := range pipeline.TaskDependencies(taskName) {
		if _, ok := ctx.Tasks[depName]; ok {
			continue
		}

		dep, err := pipeline.Task(depName)
		if err != nil {
			continue
		}

		deps = append(deps, dep)
	}

	return launchTasks(ctx, deps)
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
		//copyCtx.Build.State = llb.Scratch()

		copyCtx.Context = cancelCtx

		var releaseOnce sync.Once
		release := func(t TaskContext, err error) {
			releaseOnce.Do(func() {
				claim.Release(t, err)
			})
		}
		copyCtx.Context = context.WithValue(copyCtx.Context, selfReleaseKey{}, release)

		go func(claim TaskClaim) {
			t, err := next(copyCtx)
			// Fallback: releases the claim if the chain didn't already
			// release it early via releaseSelf (e.g. an error short-circuit,
			// or a task whose chain has no processor that self-releases).
			// A no-op otherwise, guarded by releaseOnce.
			release(t, err)
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
