package processor

import (
	"context"
	"errors"

	"github.com/alitto/pond/v2"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithConcurrent(pool pond.Pool) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Concurrent == nil || len(spec.Concurrent.Refs) == 0 || pool == nil {
			return nil
		}

		return &Concurrent{
			refs:          refSlice(spec.Concurrent.Refs),
			failFast:      spec.Concurrent.FailFast,
			pool:          pool,
			maxConcurrent: spec.Concurrent.MaxConcurrent,
		}
	}
}

type Concurrent struct {
	failFast      bool
	refs          []string
	pool          pond.Pool
	maxConcurrent int
}

type concurrentResult struct {
	ctx StepContext
	err error
}

func (s *Concurrent) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	steps, err := filterSteps(s.refs, pipeline)
	if err != nil {
		return nil, err
	}

	return func(ctx StepContext) (StepContext, error) {
		results := make(chan concurrentResult)
		var errs []error

		cancelCtx, cancel := context.WithCancel(ctx.Context)
		defer cancel()

		pool := s.pool
		if s.maxConcurrent > 0 {
			pool = s.pool.NewSubpool(s.maxConcurrent)
		}

		for _, step := range steps {
			next, err := step.Entrypoint()

			if err != nil {
				return ctx, err
			}

			copyCtx := ctx.DeepCopy()
			copyCtx.Context = cancelCtx
			pool.Go(func() {
				t, err := next(copyCtx)
				results <- concurrentResult{t, err}
			})
		}

		var done int
	WAIT:
		for res := range results {
			done++
			ctx = ctx.Merge(res.ctx)
			if res.err != nil && AbortOnError(res.err) {
				errs = append(errs, res.err)

				if s.failFast {
					cancel()
				}
			}

			if done == len(steps) {
				break WAIT
			}
		}

		if len(errs) > 0 {
			return ctx, errors.Join(errs...)
		}

		return next(ctx)
	}, nil
}
