package output

import (
	"context"
	"errors"
	"io"

	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/tui"
)

func UI(ui tui.UI) processor.OutputFactory {
	return func(_ context.Context, stepContext processor.StepContext, stepName string) (io.Writer, io.Writer, processor.OutputCloser) {
		task, err := ui.GetTask(stepName)
		if err != nil {
			task = tui.NewTask(stepName, stepContext.Tags)
			ui.AddTasks(task)
		}

		ui.SetStatus(tui.StepStatusRunning)
		task.SetStatus(tui.StepStatusRunning)

		return task, task, func(err error) error {
			switch {
			case err == nil:
				task.SetStatus(tui.StepStatusDone)
			case errors.Is(err, processor.ErrAllowFailure):
				task.SetStatus(tui.StepStatusSkipped)
			case errors.Is(err, processor.ErrConditionFalse):
				task.SetStatus(tui.StepStatusSkipped)
			case errors.Is(err, processor.ErrSkipDone):
				task.SetStatus(tui.StepStatusSkipped)
			default:
				task.SetStatus(tui.StepStatusFailed)
			}

			return nil
		}
	}
}
