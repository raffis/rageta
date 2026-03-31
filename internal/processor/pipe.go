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
	next       Next
	r          io.ReadCloser
	w          io.WriteCloser
	lastStdout io.Reader
	ctx        StepContext
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

		results := make(chan pipeResult)
		var stdout *io.PipeReader
		var errs []error

		for i := range stepEntrypoints {
			copyCtx := ctx.DeepCopy()

			if len(steps) == i+1 {
				copyCtx.Streams.Stdin = stdout
			} else {
				if !s.tee {
					copyCtx.Streams.Stdout = io.Discard
				}

				r, w := io.Pipe()
				stepEntrypoints[i].r = r
				stepEntrypoints[i].w = w

				if i > 0 {
					stepEntrypoints[i].lastStdout = stepEntrypoints[i-1].r
				}

				if stdout != nil {
					copyCtx.Streams.Stdin = stdout
				}

				copyCtx.Streams.AdditionalStdout = append(copyCtx.Streams.AdditionalStdout, w)
				stdout = r
			}

			stepEntrypoints[i].ctx = copyCtx
		}

		cancelCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		for _, step := range stepEntrypoints {
			step.ctx.Context = cancelCtx

			go func() {
				resultCtx, err := step.next(step.ctx)
				// Normal pipe stages must close their own writer immediately so downstream readers
				// can observe EOF. ErrConditionFalse stages are handled by forwarding logic below.
				if step.w != nil && !errors.Is(err, ErrConditionFalse) {
					if closeErr := step.w.Close(); closeErr != nil {
						err = closeErr
					}
				}
				results <- pipeResult{resultCtx, err, step.w, step.lastStdout}
			}()
		}

		var done int
	WAIT:
		for res := range results {
			done++
			ctx = ctx.Merge(res.ctx)
			switch {
			case res.err != nil && AbortOnError(res.err):
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
			// If one pipe step is skipped instead aborting the pipe the streams from the previous step
			// are passed to the next after the skipped one
			case errors.Is(res.err, ErrConditionFalse):
				if res.nextStdin != nil && res.lastStdout != nil {
					_, copyErr := io.Copy(res.nextStdin, res.lastStdout)
					if copyErr != nil {
						errs = append(errs, copyErr)
					}
				}
				if res.nextStdin != nil {
					_ = res.nextStdin.Close()
				}
			default:
				if res.nextStdin != nil {
					_ = res.nextStdin.Close()
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

type pipeResult struct {
	ctx        StepContext
	err        error
	nextStdin  io.WriteCloser
	lastStdout io.Reader
}
