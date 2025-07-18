package processor

import (
	"context"
	"errors"
	"io"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithPipe(tee bool) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Pipe == nil || len(spec.Pipe.Refs) == 0 {
			return nil
		}

		return &Pipe{
			refs: refSlice(spec.Pipe.Refs),
			tee:  tee,
		}
	}
}

type Pipe struct {
	refs []string
	tee  bool
}

type stepWrapper struct {
	next Next
	r    io.ReadCloser
	w    io.WriteCloser
	ctx  StepContext
}

func (s *Pipe) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	steps, err := filterSteps(s.refs, pipeline)
	if err != nil {
		return nil, err
	}

	return func(ctx StepContext) (StepContext, error) {
		var stepEntrypoints []stepWrapper
		for _, step := range steps {
			entrypoint, err := step.Entrypoint()

			if err != nil {
				return ctx, err
			}

			stepEntrypoints = append(stepEntrypoints, stepWrapper{
				next: entrypoint,
			})
		}

		results := make(chan result)
		var stdout *io.PipeReader
		var errs []error

		for i := range stepEntrypoints {
			copyCtx := ctx.DeepCopy()

			if len(steps) == i+1 {
				copyCtx.Stdin = stdout
			} else {
				if !s.tee {
					copyCtx.Stdout = io.Discard
				}

				r, w := io.Pipe()
				stepEntrypoints[i].r = r
				stepEntrypoints[i].w = w

				if stdout != nil {
					copyCtx.Stdin = stdout
				}

				copyCtx.AdditionalStdout = append(copyCtx.AdditionalStdout, w)
				stdout = r
			}

			stepEntrypoints[i].ctx = copyCtx
		}

		cancelCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		for _, step := range stepEntrypoints {
			step := step
			step.ctx.Context = cancelCtx

			go func() {
				resultCtx, err := step.next(step.ctx)
				if step.r != nil {
					if closeErr := step.r.Close(); closeErr != nil {
						err = closeErr
					}
				}
				results <- result{resultCtx, err}
			}()
		}

		var done int
	WAIT:
		for res := range results {
			done++
			ctx = ctx.Merge(res.ctx)
			if res.err != nil && AbortOnError(res.err) {
				errs = append(errs, res.err)
				cancel()

				//close any open io pipe to make any std stream copy routines stop
				for _, step := range stepEntrypoints[0 : len(steps)-1] {
					if step.r != nil {
						_ = step.r.Close()
					}
					if step.w != nil {
						_ = step.w.Close()
					}
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
