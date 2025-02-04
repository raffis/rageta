package output

import (
	"errors"
	"io"

	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/tui"
)

func UI(ui tui.UI) processor.OutputFactory {
	return func(name string, stdin io.Reader, stdout, stderr io.Writer) (io.Reader, io.Writer, io.Writer, processor.OutputCloser) {
		task, err := ui.GetTask(name)
		if err != nil {
			task = tui.NewTask(name)
			ui.AddTasks(task)
		}

		ui.SetStatus(tui.StepStatusRunning)
		task.SetStatus(tui.StepStatusRunning)

		return stdin, task, task, func(err error) {
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
		}
	}
}
