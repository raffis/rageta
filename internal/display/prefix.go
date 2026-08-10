package display

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/styles"
	"github.com/raffis/rageta/internal/utils"
	"github.com/raffis/rageta/internal/xio"
)

func Prefix(stdout, stderr io.Writer, eventsInterval time.Duration) processor.DisplayFactory {
	return func(ctx processor.TaskContext, stepName, short string) processor.Display {
		prefix := fmt.Appendf(nil, "%s ", ctx.Style.Style.Render(ctx.UniqueName()))
		d := &prefixDisplay{
			stdout:         xio.NewLineWriter(xio.NewPrefixWriter(stdout, prefix)),
			eventsInterval: eventsInterval,
		}
		if stdout == stderr {
			d.stderr = d.stdout
		} else {
			d.stderr = xio.NewLineWriter(xio.NewPrefixWriter(stderr, prefix))
		}

		d.events = xio.NewLineWriter(xio.NewPrefixWriter(xio.NewLipglossWriter(d.stderr, styles.Highlight), []byte("➤ ")))
		d.startPolling(ctx)

		return d
	}
}

type prefixDisplay struct {
	stdout, stderr *xio.LineWriter
	events         io.Writer
	eventsInterval time.Duration
	quit           chan struct{}
}

func (d *prefixDisplay) Stdout() io.Writer {
	return d.stdout
}

func (d *prefixDisplay) Stderr() io.Writer {
	return d.stderr
}

func (d *prefixDisplay) Events() io.Writer {
	return d.events
}

func (d *prefixDisplay) startPolling(ctx processor.TaskContext) error {
	if ctx.StartedAt.IsZero() {
		return errors.New("step not started, missing startedAt")
	}

	if d.eventsInterval > 0 {
		d.quit = make(chan struct{})
		ticker := time.NewTicker(d.eventsInterval)

		go func() {
			for {
				select {
				case <-ticker.C:
					duration := time.Since(ctx.StartedAt).Round(time.Millisecond * 100)
					_, _ = fmt.Fprintf(d.events, "Waiting for %q to finish [%s]\n", ctx.UniqueName(), duration)
				case <-d.quit:
					ticker.Stop()
					return
				}
			}
		}()
	}

	_, _ = fmt.Fprintf(d.events, "Task %q started\n", ctx.UniqueName())
	return nil
}

func (d *prefixDisplay) Close(ctx processor.TaskContext, err error) error {
	if d.quit != nil {
		close(d.quit)
	}

	duration := time.Since(ctx.StartedAt).Round(time.Millisecond * 100)
	switch {
	case err == nil && ctx.Build.Cached:
		_, _ = fmt.Fprintf(d.events, "Task %q cached [%s]\n", ctx.UniqueName(), duration)
	case err == nil:
		_, _ = fmt.Fprintf(d.events, "Task %q done [%s]\n", ctx.UniqueName(), duration)
	case errors.Is(err, processor.ErrAllowFailure):
		_, _ = fmt.Fprintf(d.events, "Task %q failed and pipeline is continued [%s]\n", ctx.UniqueName(), duration)
	case errors.Is(err, processor.ErrConditionFalse):
		_, _ = fmt.Fprintf(d.events, "Task %q condition check did not pass [%s]\n", ctx.UniqueName(), duration)
	default:
		_, _ = fmt.Fprintf(d.events, "Task %q failed: %q [%s]\n", ctx.UniqueName(), err.Error(), duration)
	}

	if err := d.stdout.Flush(); err != nil {
		return fmt.Errorf("error flushing stdout: %w", err)
	}
	if err := d.stderr.Flush(); err != nil {
		return fmt.Errorf("error flushing stderr: %w", err)
	}
	return nil
}

func (d *prefixDisplay) WriteStats(cpu, mem, netRx, netTx int64) error {
	fmt.Fprintf(d.events, "STATS: cpu=%dm mem=%s net_rx=%s net_tx=%s\n", cpu, utils.FormatBytes(mem), utils.FormatBps(netRx), utils.FormatBps(netTx))
	return nil
}

func (d *prefixDisplay) WritePullProgress(current, total int64) error {
	fmt.Fprintf(d.events, "pulling %s/%s\n", utils.FormatBytes(current), utils.FormatBytes(total))
	return nil
}
