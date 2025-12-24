package processor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

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
		var fifoPaths []string
		var errs []error

		// Create FIFO pipes for each step except the last one
		// Use NamePrefix to make FIFOs unique per matrix iteration
		fifoPrefix := "pipe"
		if ctx.NamePrefix != "" {
			fifoPrefix = fmt.Sprintf("pipe-%s", ctx.NamePrefix)
		}
		for i := range len(steps) - 1 {
			fifoPath := filepath.Join(ctx.Dir, fmt.Sprintf("%s-%d.fifo", fifoPrefix, i))
			if err := os.MkdirAll(filepath.Dir(fifoPath), 0755); err != nil {
				return ctx, fmt.Errorf("failed to create fifo directory: %w", err)
			}

			if err := os.Remove(fifoPath); err != nil && !os.IsNotExist(err) {
				return ctx, fmt.Errorf("failed to remove existing fifo: %w", err)
			}

			if err := syscall.Mkfifo(fifoPath, 0644); err != nil {
				return ctx, fmt.Errorf("failed to create fifo: %w", err)
			}

			fifoPaths = append(fifoPaths, fifoPath)
		}

		for i := range stepEntrypoints {
			copyCtx := ctx.DeepCopy()

			if len(steps) == i+1 {
				if len(fifoPaths) > 0 {
					copyCtx.StdinPath = fifoPaths[len(fifoPaths)-1]
				}
			} else {
				if !s.tee {
					copyCtx.Stdout = nil
				}

				copyCtx.AdditionalStdoutPaths = append(copyCtx.AdditionalStdoutPaths, fifoPaths[i])

				// If not the first step, read from previous FIFO
				if i > 0 {
					copyCtx.StdinPath = fifoPaths[i-1]
				}
			}

			stepEntrypoints[i].ctx = copyCtx
		}

		cancelCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		defer func() {
			for _, fifoPath := range fifoPaths {
				_ = os.Remove(fifoPath)
			}
		}()

		for i := range stepEntrypoints {
			stepEntrypoints[i].ctx.Context = cancelCtx

			go func(idx int) {
				step := stepEntrypoints[idx]
				resultCtx, err := step.next(step.ctx)
				results <- result{resultCtx, err}
			}(i)
		}

		var done int
	WAIT:
		for res := range results {
			done++
			ctx = ctx.Merge(res.ctx)
			if res.err != nil && AbortOnError(res.err) {
				errs = append(errs, res.err)
				cancel()
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
