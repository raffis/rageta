package processor

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/raffis/rageta/internal/styles"
	"github.com/raffis/rageta/internal/xio"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithEvents(enabled bool, interval time.Duration, dev io.Writer) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if !enabled {
			return nil
		}

		return &Events{
			stepName: spec.Name,
			ticker:   time.NewTicker(interval),
			dev:      dev,
		}
	}
}

type Events struct {
	stepName string
	ticker   *time.Ticker
	dev      io.Writer
}

type EventsContext struct {
	Dev io.Writer
}

func newEventsContext() EventsContext {
	return EventsContext{
		Dev: io.Discard,
	}
}

func (s *Events) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		if ctx.StartedAt.IsZero() {
			return ctx, errors.New("step not started, missing startedAt")
		}

		origDev := ctx.Events.Dev

		switch {
		case s.dev != nil:
			ctx.Events.Dev = s.dev
		case ctx.Streams.Stderr != nil && ctx.Streams.Stderr != io.Discard:
			ctx.Events.Dev = ctx.Streams.Stderr
		default:
			return next(ctx)
		}

		quit := make(chan struct{})
		defer func() {
			quit <- struct{}{}
		}()

		ctx.Events.Dev = xio.NewLineWriter(xio.NewPrefixWriter(xio.NewLipglossWriter(ctx.Events.Dev, styles.Highlight), []byte("➤ ")))
		progress := func() {
			duration := time.Since(ctx.StartedAt).Round(time.Millisecond * 100)
			_, _ = fmt.Fprintf(ctx.Events.Dev, "Waiting for %q to finish [%s]\n", ctx.UniqueName(), duration)
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

		_, _ = fmt.Fprintf(ctx.Events.Dev, "Task %q started\n", ctx.UniqueName())
		ctx, err := next(ctx)
		duration := time.Since(ctx.StartedAt).Round(time.Millisecond * 100)

		switch {
		case err == nil:
			_, _ = fmt.Fprintf(ctx.Events.Dev, "Task %q done [%s]\n", ctx.UniqueName(), duration)
		case errors.Is(err, ErrAllowFailure):
			_, _ = fmt.Fprintf(ctx.Events.Dev, "Task %q failed and pipeline is continued [%s]\n", ctx.UniqueName(), duration)
		case errors.Is(err, ErrConditionFalse):
			_, _ = fmt.Fprintf(ctx.Events.Dev, "Task %q condition check did not pass [%s]\n", ctx.UniqueName(), duration)
		case errors.Is(err, ErrSkipDone):
			_, _ = fmt.Fprintf(ctx.Events.Dev, "Task %q skipped as it was marked as done [%s]\n", ctx.UniqueName(), duration)
		default:
			_, _ = fmt.Fprintf(ctx.Events.Dev, "Task %q failed: %q [%s]\n", ctx.UniqueName(), err.Error(), duration)
		}

		ctx.Events.Dev = origDev
		return ctx, err
	}, nil
}
