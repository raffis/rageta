package display

import (
	"errors"
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"
	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/tui"
)

type sender interface {
	Send(msg tea.Msg)
}

func UI(sender sender) processor.DisplayFactory {
	return func(ctx processor.TaskContext, stepName, short string) (io.Writer, io.Writer, processor.DisplayCloser) {
		displayName := stepName
		if short != "" {
			displayName = short
		}

		uniqueName := ctx.UniqueName()
		step := tui.NewTask()
		step.Name = uniqueName
		step.DisplayName = displayName
		step.Labels = ctx.Labels.Labels()
		step.Status = tui.TaskStatusRunning
		sender.Send(step)

		return step, step, func(err error) error {
			if err := step.Flush(); err != nil {
				return fmt.Errorf("error flushing stdout: %w", err)
			}

			switch {
			case err == nil:
				sender.Send(tui.TaskMsg{
					Name:   uniqueName,
					Status: tui.TaskStatusDone,
				})
			case errors.Is(err, processor.ErrAllowFailure):
				sender.Send(tui.TaskMsg{
					Name:   uniqueName,
					Status: tui.TaskStatusSkipped,
				})
			case errors.Is(err, processor.ErrConditionFalse):
				sender.Send(tui.TaskMsg{
					Name:   uniqueName,
					Status: tui.TaskStatusSkipped,
				})
			default:
				sender.Send(tui.TaskMsg{
					Name:   uniqueName,
					Status: tui.TaskStatusFailed,
				})
			}

			return nil
		}
	}
}
