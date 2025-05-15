package report

import (
	"strings"
	"time"

	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/tui"
)

func stringify(step processor.StepContext) (string, string, string) {
	var (
		duration time.Duration
		status   tui.StepStatus
		errMsg   string
	)

	switch {
	case step.StartedAt.IsZero():
		status = tui.StepStatusWaiting
	case step.Error != nil && !processor.AbortOnError(step.Error):
		status = tui.StepStatusSkipped
		errMsg = strings.ReplaceAll(step.Error.Error(), "\n", "")
	case step.Error != nil:
		status = tui.StepStatusFailed
		errMsg = strings.ReplaceAll(step.Error.Error(), "\n", "")
	case step.Error == nil:
		status = tui.StepStatusDone
	}

	if !step.EndedAt.IsZero() {
		duration = step.EndedAt.Sub(step.StartedAt).Round(time.Millisecond * 10)
	}

	return errMsg, status.Render(), duration.String()
}
