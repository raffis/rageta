package processor

import (
	"context"
	"errors"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithDependsOn() ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		/*if len(spec.DependsOn) == 0 {
			return nil
		}*/

		return &DependsOn{
			refs:     refSlice(spec.DependsOn),
			stepName: spec.Name,
		}
	}
}

type DependsOn struct {
	refs     []string
	stepName string
}

func (s *DependsOn) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		var dependsOn []Task

		for _, name := range s.refs {
			_, started := ctx.Tasks[name]
			if started {
				continue
			}

			step, err := pipeline.Task(name)
			if err != nil {
				return ctx, err
			}

			dependsOn = append(dependsOn, step)
		}

		ctx, err := s.processTasks(ctx, dependsOn)

		if err != nil {
			return ctx, err
		}

		ctx, err = next(ctx)
		if err != nil {
			return ctx, err
		}

		var ready []Task
		for _, candidate := range pipeline.DependantTasks(s.stepName) {
			allSatisfied := true
			for _, dep := range pipeline.TaskDependencies(candidate.Name()) {
				if dep == s.stepName {
					continue
				}
				if _, ok := ctx.Tasks[dep]; !ok {
					allSatisfied = false
					break
				}
			}
			if allSatisfied {
				ready = append(ready, candidate)
			}
		}

		return s.processTasks(ctx, ready)
	}, nil
}

func (s *DependsOn) processTasks(ctx TaskContext, steps []Task) (TaskContext, error) {
	if len(steps) == 0 {
		return ctx, nil
	}

	results := make(chan result)
	var errs []error

	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var launched int
	for _, step := range steps {
		if _, alreadyRunning := ctx.Tasks[step.Name()]; alreadyRunning {
			continue
		}

		next, err := step.Entrypoint()
		if err != nil {
			return ctx, err
		}

		launched++
		copyCTX := ctx.DeepCopy()
		copyCTX.Context = cancelCtx
		copyCTX.Tasks[s.stepName] = &copyCTX

		go func() {
			t, err := next(copyCTX)
			results <- result{t, err}
		}()
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
