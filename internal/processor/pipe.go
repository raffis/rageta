package processor

import (
	"context"
	"errors"
	"io"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithPipe() ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Pipe == nil || len(spec.Pipe.Refs) == 0 {
			return nil
		}

		return &Pipe{
			refs: refSlice(spec.Pipe.Refs),
		}
	}
}

type Pipe struct {
	refs []string
}

type stepWrapper struct {
	next        Next
	r           io.ReadCloser
	w           io.WriteCloser
	stepContext StepContext
}

func (s *Pipe) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	steps, err := filterSteps(s.refs, pipeline)
	if err != nil {
		return nil, err
	}

	var stepEntrypoints []stepWrapper
	for _, step := range steps {
		entrypoint, err := step.Entrypoint()

		if err != nil {
			return nil, err
		}

		stepEntrypoints = append(stepEntrypoints, stepWrapper{
			next: entrypoint,
		})
	}

	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		results := make(chan concurrentResult)
		var stdout *io.PipeReader
		var errs []error

		for i := range stepEntrypoints {
			copy := stepContext.DeepCopy()

			if len(steps) == i+1 {
				copy.Stdin = stdout
			} else {
				r, w := io.Pipe()
				stepEntrypoints[i].r = r
				stepEntrypoints[i].w = w

				if stdout != nil {
					copy.Stdin = stdout
				}

				copy.Stdout.Add(w)
				stdout = r
			}

			stepEntrypoints[i].stepContext = copy
		}

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		for _, step := range stepEntrypoints {
			step := step

			go func() {
				resultCtx, err := step.next(ctx, step.stepContext)
				if step.r != nil {
					step.r.Close()
				}

				results <- concurrentResult{resultCtx, err}
			}()
		}

		var done int
	WAIT:
		for res := range results {
			done++
			stepContext = stepContext.Merge(res.stepContext)
			if res.err != nil && AbortOnError(res.err) {
				errs = append(errs, res.err)
				cancel()
			}

			if done == len(steps) {
				break WAIT
			}
		}

		if len(errs) > 0 {
			return stepContext, errors.Join(errs...)
		}

		return next(ctx, stepContext)
	}, nil
}
