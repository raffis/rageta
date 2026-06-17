package processor

import (
	"context"
	"errors"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithDependsOn() ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
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
	return func(ctx StepContext) (StepContext, error) {
		var dependsOn []Step

		for _, name := range s.refs {
			_, started := ctx.Steps[name]
			if started {
				continue
			}

			step, err := pipeline.Step(name)
			if err != nil {
				return ctx, err
			}

			dependsOn = append(dependsOn, step)
		}

		ctx, err := s.processSteps(ctx, dependsOn)

		if err != nil {
			return ctx, err
		}

		ctx, err = next(ctx)
		if err != nil {
			return ctx, err
		}

		return ctx, nil
		//return s.processSteps(ctx, pipeline.DependantSteps(s.stepName))
	}, nil
}

func (s *DependsOn) processSteps(ctx StepContext, steps []Step) (StepContext, error) {
	if len(steps) == 0 {
		return ctx, nil
	}

	results := make(chan result)
	var errs []error

	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var launched int
	for _, step := range steps {
		if _, alreadyRunning := ctx.Steps[step.Name()]; alreadyRunning {
			continue
		}

		next, err := step.Entrypoint()
		if err != nil {
			return ctx, err
		}

		launched++
		copyCTX := ctx.DeepCopy()
		copyCTX.Context = cancelCtx
		copyCTX.Steps[s.stepName] = &copyCTX

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
