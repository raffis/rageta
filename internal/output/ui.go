package output

import (
	"errors"
	"io"

  tea "charm.land/bubbletea/v2"
	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/tui"
)

type sender interface {
	Send(msg tea.Msg)
}

func UI(sender sender) processor.OutputFactory {
	return func(ctx processor.StepContext, stepName, short string) (io.Writer, io.Writer, processor.OutputCloser) {
		uniqueName := processor.SuffixName(stepName, ctx.NamePrefix)
		displayName := stepName
		if short != "" {
			displayName = short
		}

		step := tui.NewStep()
		step.Name = uniqueName
		step.DisplayName = displayName
		step.Tags = ctx.Tags()
		step.Status = tui.StepStatusRunning

		sender.Send(step)

		return step, step, func(err error) error {
			switch {
			case err == nil:
				sender.Send(tui.StepMsg{
					Name:   uniqueName,
					Status: tui.StepStatusDone,
				})
			case errors.Is(err, processor.ErrAllowFailure):
				sender.Send(tui.StepMsg{
					Name:   uniqueName,
					Status: tui.StepStatusSkipped,
				})
			case errors.Is(err, processor.ErrConditionFalse):
				sender.Send(tui.StepMsg{
					Name:   uniqueName,
					Status: tui.StepStatusSkipped,
				})
			case errors.Is(err, processor.ErrSkipDone):
				sender.Send(tui.StepMsg{
					Name:   uniqueName,
					Status: tui.StepStatusSkipped,
				})
			default:
				sender.Send(tui.StepMsg{
					Name:   uniqueName,
					Status: tui.StepStatusFailed,
				})
			}

			return nil
		}
	}
}
