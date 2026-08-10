package display

import (
	"errors"
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"
	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/styles"
	"github.com/raffis/rageta/internal/tui"
	"github.com/raffis/rageta/internal/xio"
)

func UI(sender sender) processor.DisplayFactory {
	return func(ctx processor.TaskContext, stepName, short string) processor.Display {
		displayName := stepName
		if short != "" {
			displayName = short
		}

		uniqueName := ctx.UniqueName()
		step := tui.NewTask()
		step.Name = uniqueName
		step.DisplayName = displayName
		step.Labels = ctx.Labels.Labels()
		step.Status = tui.TaskStatusWaiting
		sender.Send(step)
		events := xio.NewLineWriter(xio.NewPrefixWriter(xio.NewLipglossWriter(step, styles.Highlight), []byte("➤ ")))

		return &uiDisplay{step: step, uniqueName: uniqueName, sender: sender, events: events}
	}
}

type sender interface {
	Send(msg tea.Msg)
}

type uiDisplay struct {
	step       tui.TaskMsg
	uniqueName string
	sender     sender
	events     io.Writer
}

func (d *uiDisplay) Stdout() io.Writer {
	return &d.step
}

func (d *uiDisplay) Stderr() io.Writer {
	return &d.step
}

func (d *uiDisplay) Events() io.Writer {
	return d.events
}

func (d *uiDisplay) Close(ctx processor.TaskContext, err error) error {
	if err := d.step.Flush(); err != nil {
		return fmt.Errorf("error flushing stdout: %w", err)
	}
	taskCtx := ctx.DeepCopy()

	switch {
	case err == nil:
		status := tui.TaskStatusDone
		if ctx.Build.Cached {
			status = tui.TaskStatusCached
		}
		d.sender.Send(tui.TaskMsg{Name: d.uniqueName, Status: status, Context: taskCtx})
	case errors.Is(err, processor.ErrAllowFailure):
		d.sender.Send(tui.TaskMsg{Name: d.uniqueName, Status: tui.TaskStatusSkipped, Context: taskCtx})
	case errors.Is(err, processor.ErrConditionFalse):
		d.sender.Send(tui.TaskMsg{Name: d.uniqueName, Status: tui.TaskStatusSkipped, Context: taskCtx})
	default:
		fmt.Fprintf(d.events, "Task %q failed: %q\n", ctx.UniqueName(), err.Error())
		d.sender.Send(tui.TaskMsg{Name: d.uniqueName, Status: tui.TaskStatusFailed, Context: taskCtx})
	}

	return nil
}

func (d *uiDisplay) WriteStats(cpu, mem, netRx, netTx int64) error {
	d.sender.Send(tui.ResourceStatsMsg{
		Name:  d.uniqueName,
		Stats: tui.ResourceStats{CPUMillicores: cpu, MemBytes: mem, NetRxBytes: netRx, NetTxBytes: netTx},
	})

	return nil
}

func (d *uiDisplay) WritePullProgress(current, total int64) error {
	d.sender.Send(tui.PullProgressMsg{
		Name:    d.uniqueName,
		Current: current,
		Total:   total,
	})

	return nil
}
