package processor

import (
	"context"
	"errors"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithDependsOn() ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		return &DependsOn{
			refs:     spec.DependsOn,
			taskName: spec.Name,
		}
	}
}

type DependsOn struct {
	refs     []v1beta1.TaskReference
	taskName string
}

func (s *DependsOn) resolveRef(pipeline Pipeline, ref v1beta1.TaskReference) ([]Task, error) {
	switch {
	case ref.Name != nil:
		task, err := pipeline.Task(*ref.Name)
		if err != nil {
			return nil, err
		}
		return []Task{task}, nil
	case ref.MatchLabels != nil:
		return pipeline.TasksByLabels(ref.MatchLabels), nil
	default:
		return nil, errors.New("invalid task reference")
	}
}

func (s *DependsOn) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		var dependsOn []Task

		for _, ref := range s.refs {
			tasks, err := s.resolveRef(pipeline, ref)
			if err != nil {
				return ctx, err
			}

			for _, task := range tasks {
				if _, started := ctx.Tasks[task.Name()]; started {
					continue
				}
				dependsOn = append(dependsOn, task)
			}
		}

		ctx, err := s.processTasks(ctx, dependsOn)

		if err != nil {
			return ctx, err
		}

		return next(ctx)
	}, nil
}

func (s *DependsOn) processTasks(ctx TaskContext, tasks []Task) (TaskContext, error) {
	if len(tasks) == 0 {
		return ctx, nil
	}

	results := make(chan result)
	var errs []error

	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var launched int
	for _, task := range tasks {
		/*if _, alreadyRunning := ctx.Tasks[task.Name()]; alreadyRunning {
			continue
		}*/

		claim, owner := task.Claim(ctx)
		launched++

		if !owner {
			go func(claim TaskClaim) {
				t, err := claim.Wait()

				copyCTX := t.DeepCopy()
				copyCTX.Context = cancelCtx
				copyCTX.Tasks[s.taskName] = &copyCTX

				results <- result{copyCTX, err}
			}(claim)
			continue
		}

		next, err := task.Entrypoint()
		if err != nil {
			claim.Release(ctx.DeepCopy(), err)
			return ctx, err
		}

		copyCTX := ctx.DeepCopy()
		copyCTX.Context = cancelCtx
		copyCTX.Tasks[s.taskName] = &copyCTX

		go func(claim TaskClaim) {
			t, err := next(copyCTX)
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
