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

		progress := func(i int) {
			dev.Write([]byte(styles.Highlight.Render(fmt.Sprintf("=> Waiting for %q to finish [%d]", s.stepName, i))))
			dev.Write([]byte("\n"))
		}

		go func() {
			i := 0
			for {
				select {
				case <-s.ticker.C:
					progress(i)
				case <-quit:
					s.ticker.Stop()
					return
				}

				i++
			}
		}()

		dev.Write([]byte(styles.Highlight.Render(fmt.Sprintf("=> Task %q started", s.stepName))))
		dev.Write([]byte("\n"))

		ctx, err := next(ctx)

		switch {
		case err == nil:
			dev.Write([]byte(styles.Highlight.Render(fmt.Sprintf("=> Task %q done", s.stepName))))
		case errors.Is(err, ErrAllowFailure):
			dev.Write([]byte(styles.Highlight.Render(fmt.Sprintf("=> Task %q failed and pipeline is continued", s.stepName))))
		case errors.Is(err, ErrConditionFalse):
			dev.Write([]byte(styles.Highlight.Render(fmt.Sprintf("=> Task %q condition check did not pass", s.stepName))))
		case errors.Is(err, ErrSkipDone):
			dev.Write([]byte(styles.Highlight.Render(fmt.Sprintf("=> Task %q skipped as it was marked as done", s.stepName))))
		default:
			dev.Write([]byte(styles.Highlight.Render(fmt.Sprintf("=> Task %q failed: %q", s.stepName, err.Error()))))
		}
		dev.Write([]byte("\n"))

		return ctx, err
	}, nil
}
