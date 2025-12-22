package processor

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/raffis/rageta/internal/styles"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithMonitor(enabled bool, interval time.Duration, dev io.Writer) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if !enabled {
			return nil
		}

		return &Monitor{
			stepName: spec.Name,
			ticker:   time.NewTicker(interval),
			dev:      dev,
		}
	}
}

const prompt = `➤`

type Monitor struct {
	stepName string
	ticker   *time.Ticker
	dev      io.Writer
}

func (s *Monitor) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		if ctx.StartedAt.IsZero() {
			return ctx, errors.New("step not started, missing startedAt")
		}

		var dev io.Writer

		switch {
		case s.dev != nil:
			dev = s.dev
		case ctx.Stderr != nil && ctx.Stderr != io.Discard:
			dev = ctx.Stderr
		default:
			return next(ctx)
		}

		quit := make(chan struct{})
		defer func() {
			quit <- struct{}{}
		}()

		stepName := SuffixName(s.stepName, ctx.NamePrefix)

		progress := func() {
			duration := time.Since(ctx.StartedAt).Round(time.Millisecond * 100)
			_, _ = dev.Write([]byte(styles.Highlight.Render(fmt.Sprintf("%s Waiting for %q to finish [%s]", prompt, stepName, duration)) + "\n"))
		}

		go func() {
			for {
				select {
				case <-s.ticker.C:
					progress()
				case <-quit:
					s.ticker.Stop()
					return
				}
			}
		}()

		_, _ = dev.Write([]byte(styles.Highlight.Render(fmt.Sprintf("%s Task %q started", prompt, stepName)) + "\n"))

		ctx, err := next(ctx)
		duration := time.Since(ctx.StartedAt).Round(time.Millisecond * 100)

		switch {
		case err == nil:
			_, _ = dev.Write([]byte(styles.Highlight.Render(fmt.Sprintf("%s Task %q done [%s]", prompt, stepName, duration)) + "\n"))
		case errors.Is(err, ErrAllowFailure):
			_, _ = dev.Write([]byte(styles.Highlight.Render(fmt.Sprintf("%s Task %q failed and pipeline is continued [%s]", prompt, stepName, duration)) + "\n"))
		case errors.Is(err, ErrConditionFalse):
			_, _ = dev.Write([]byte(styles.Highlight.Render(fmt.Sprintf("%s Task %q condition check did not pass [%s]", prompt, stepName, duration)) + "\n"))
		case errors.Is(err, ErrSkipDone):
			_, _ = dev.Write([]byte(styles.Highlight.Render(fmt.Sprintf("%s Task %q skipped as it was marked as done [%s]", prompt, stepName, duration)) + "\n"))
		default:
			_, _ = dev.Write([]byte(styles.Highlight.Render(fmt.Sprintf("%s Task %q failed: %q [%s]", prompt, stepName, err.Error(), duration)) + "\n"))
		}

		return ctx, err
	}, nil
}
