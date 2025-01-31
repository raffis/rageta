package report

import (
	"strings"
	"time"

	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/tui"
)

func stringify(step stepResult) (string, string, string) {
	var (
		duration time.Duration
		status   tui.StepStatus
		errMsg   string
	)

	switch {
	case step.result == nil:
		status = tui.StepStatusWaiting
	case step.result.Error != nil && !processor.AbortOnError(step.result.Error):
		status = tui.StepStatusSkipped
		errMsg = strings.ReplaceAll(step.result.Error.Error(), "\n", "")
	case step.result.Error != nil:
		status = tui.StepStatusFailed
		errMsg = strings.ReplaceAll(step.result.Error.Error(), "\n", "")
	case step.result.Error == nil:
		status = tui.StepStatusDone
	}

	if step.result != nil {
		duration = step.result.Duration().Round(time.Millisecond * 10)
	}

	return errMsg, status.Render(), duration.String()
}
