package processor

import (
	"context"
	"io"

	"github.com/hashicorp/go-multierror"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithPipe() ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Pipe == nil {
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
		result := &multierror.Error{}
		results := make(chan concurrentResult)
		var errs []error
		var stdout *io.PipeReader

		for i := range stepEntrypoints {
			copy := stepContext.DeepCopy()

			if len(steps) == i+1 {
				copy.Stdin = stdout
			} else {
				r, w := io.Pipe()
				stepEntrypoints[i].r = r

				if stdout != nil {
					copy.Stdin = stdout
				}

				copy.Stdout = w
				stdout = r
			}

			stepEntrypoints[i].stepContext = copy
		}

		for _, step := range stepEntrypoints {
			step := step

			go func() {
				t, err := step.next(ctx, step.stepContext)
				if step.r != nil {
					step.r.Close()
				}

				results <- concurrentResult{t, err}
			}()
		}

		var done int
	WAIT:
		for {
			select {
			case <-ctx.Done():
				return stepContext, nil
			case res := <-results:
				done++
				stepContext = stepContext.Merge(res.stepContext)
				if res.err != nil {
					errs = append(errs, res.err)

					if err != nil {
						break WAIT
					}
				}

				if done == len(steps) {
					break WAIT
				}
			}
		}

		multierror.Append(result, errs...)
		if len(result.Errors) > 0 {
			return stepContext, result
		}

		return next(ctx, stepContext)
	}, nil
}
