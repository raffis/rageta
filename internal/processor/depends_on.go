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

		return &Needs{
			refs:     refSlice(spec.DependsOn),
			stepName: spec.Name,
		}
	}
}

type Needs struct {
	refs     []string
	stepName string
}

func (s *Needs) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
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

		//fmt.Printf("run before NEXT %s - %#v - %#v\n", s.stepName, s.refs, ctx.Steps)

		ctx, err := s.processSteps(ctx, dependsOn)
		if err != nil {
			return ctx, err
		}

		//	fmt.Printf("run next %s\n", s.stepName)

		//for x, x2 := range ctx.Steps {
		//	fmt.Printf("== %#v -- %#v\n", x, x2.LLBState)
		//}

		ctx, err = next(ctx)
		//fmt.Printf("finished next %s -- %#v\n", s.stepName, ctx.LLBState)
		//for x, x2 := range ctx.Steps {
		//	fmt.Printf("== %#v -- %#v\n", x, x2.LLBState)
		//}

		if err != nil {
			return ctx, err
		}

		//fmt.Printf("AFTER %#v\n", pipeline.DependantSteps(s.stepName))

		return s.processSteps(ctx, pipeline.DependantSteps(s.stepName))
	}, nil
}

func (s *Needs) processSteps(ctx StepContext, steps []Step) (StepContext, error) {
	if len(steps) == 0 {
		return ctx, nil
	}

	results := make(chan result)
	var errs []error

	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, step := range steps {
		next, err := step.Entrypoint()
		if err != nil {
			return ctx, err
		}

		copyCTX := ctx.DeepCopy()
		copyCTX.Context = cancelCtx
		copyCTX.Steps[s.stepName] = &copyCTX

		go func() {
			t, err := next(copyCTX)
			results <- result{t, err}
		}()
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

			//if s.failFast {
			//	cancel()
			//}
		default:
		}

		if done == len(steps) {
			break WAIT
		}
	}

	if len(errs) > 0 {
		return ctx, errors.Join(errs...)
	}

	return ctx, nil
}
