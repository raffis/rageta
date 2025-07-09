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

type Monitor struct {
	stepName string
	ticker   *time.Ticker
	dev      io.Writer
}

func (s *Monitor) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		dev := s.dev
		if ctx.Stderr != nil && ctx.Stderr != io.Discard {
			dev = ctx.Stderr
		}

		if dev == nil {
			return next(ctx)
		}

		quit := make(chan struct{})
		defer func() {
			quit <- struct{}{}
		}()

		progress := func() {
			var duration time.Duration
			if !ctx.StartedAt.IsZero() {
				duration = time.Since(ctx.StartedAt).Round(time.Millisecond * 100)
			}

			dev.Write([]byte(styles.Highlight.Render(fmt.Sprintf("=> Waiting for %q to finish [%s]", s.stepName, duration))))
			dev.Write([]byte("\n"))
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

		dev.Write([]byte(styles.Highlight.Render(fmt.Sprintf("=> Task %q started", s.stepName))))
		dev.Write([]byte("\n"))

		ctx, err := next(ctx)
		var duration time.Duration
		if !ctx.StartedAt.IsZero() {
			duration = time.Since(ctx.StartedAt).Round(time.Millisecond * 100)
		}

		switch {
		case err == nil:
			dev.Write([]byte(styles.Highlight.Render(fmt.Sprintf("=> Task %q done [%s]", s.stepName, duration))))
		case errors.Is(err, ErrAllowFailure):
			dev.Write([]byte(styles.Highlight.Render(fmt.Sprintf("=> Task %q failed and pipeline is continued [%s]", s.stepName, duration))))
		case errors.Is(err, ErrConditionFalse):
			dev.Write([]byte(styles.Highlight.Render(fmt.Sprintf("=> Task %q condition check did not pass [%s]", s.stepName, duration))))
		case errors.Is(err, ErrSkipDone):
			dev.Write([]byte(styles.Highlight.Render(fmt.Sprintf("=> Task %q skipped as it was marked as done [%s]", s.stepName, duration))))
		default:
			dev.Write([]byte(styles.Highlight.Render(fmt.Sprintf("=> Task %q failed: %q [%s]", s.stepName, err.Error(), duration))))
		}
		dev.Write([]byte("\n"))

		return ctx, err
	}, nil
}
