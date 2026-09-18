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

// selectedKey marks a task context as *selected*: the task was asked for
// explicitly — as the pipeline entrypoint, as a target of a task that ran, or
// as a dependent pushed downstream from another selected task — as opposed to
// merely being pulled in to satisfy someone else's dependsOn.
//
// Only a selected task pushes its own dependents (ChildTasks /
// AwaitMatrixChildren). Without that restriction a dependsOn edge is
// effectively bidirectional: a task pulled in implicitly would launch every
// task in the pipeline that happens to depend on it, dragging in tasks the
// entrypoint never asked for. Pushing from a selected task is still wanted —
// "run this target and everything downstream of it" — and a pushed dependent
// is itself selected, so the whole downstream closure of a target runs.
type selectedKey struct{}

// Selected marks ctx as belonging to an explicitly selected task, so that the
// task's dependents are launched once it finishes.
func Selected(ctx TaskContext) TaskContext {
	ctx.Context = context.WithValue(ctx.Context, selectedKey{}, true)
	return ctx
}

func isSelected(ctx TaskContext) bool {
	selected, _ := ctx.Value(selectedKey{}).(bool)
	return selected
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
		ctx, err := ensureDependencies(pipeline, ctx, s.taskName)
		if err != nil && AbortOnError(err) {
			return ctx, err
		}

		ctx, err = next(ctx)
		if err != nil && AbortOnError(err) {
			return ctx, err
		}

		ctx.Tasks[s.taskName] = &ctx

		// The task's own work is done and recorded; release its claim now
		releaseSelf(ctx, ctx, err)

		// Only a task that was selected pushes its dependents; a task that
		// merely got pulled in as somebody's dependency must not drag the
		// rest of its dependents into the run. See selectedKey.
		if !isSelected(ctx) {
			return ctx, err
		}

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

		childCtx, childErr := launchTasks(ctx, children, true)
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

	// Dependencies are pulled in, not selected: they run because someone
	// needs their result, so they must not push their own dependents.
	return launchTasks(ctx, deps, false)
}

// LaunchRoots claims and runs the pipeline's root tasks, each of them
// selected so that it pushes its dependents downstream as it completes.
// Going through the claim machinery (rather than starting the roots
// directly) matters: a task that depends on a root must wait for the running
// root instead of claiming it for itself and running a second copy of it.
func LaunchRoots(ctx TaskContext, tasks []Task) (TaskContext, error) {
	return launchTasks(ctx, tasks, true)
}

// launchTasks claims and runs tasks against ctx, waiting for all of them to
// finish before returning the merged context. selected records whether the
// launched tasks are themselves selected, i.e. whether they should in turn
// push their own dependents; see selectedKey.
func launchTasks(ctx TaskContext, tasks []Task, selected bool) (TaskContext, error) {
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
		copyCtx.Build.State = llb.Scratch()
		copyCtx.Context = cancelCtx

		// Set explicitly rather than inherited: ctx may itself belong to a
		// selected task, and its dependencies must not inherit that.
		copyCtx.Context = context.WithValue(copyCtx.Context, selectedKey{}, selected)

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
