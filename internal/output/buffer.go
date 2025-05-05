package output

import (
	"bytes"
	"context"
	"io"
	"sync"
	"text/template"

	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/tui"
)

type bufferVars struct {
	StepName string
	Buffer   *bytes.Buffer
	Error    error
	Symbol   string
}

func Buffer(tmpl *template.Template, stdout io.Writer) processor.OutputFactory {
	mu := sync.RWMutex{}

	return func(_ context.Context, stepContext processor.StepContext, stepName string) (io.Writer, io.Writer, processor.OutputCloser) {
		buffer := &bytes.Buffer{}

		return buffer, buffer, func(err error) error {
			mu.Lock()
			defer mu.Unlock()

			var status tui.StepStatus
			switch {
			case err == nil:
				status = tui.StepStatusWaiting
			case err != nil && !processor.AbortOnError(err):
				status = tui.StepStatusSkipped
			case err != nil:
				status = tui.StepStatusFailed
			case err == nil:
				status = tui.StepStatusDone
			}

			err = tmpl.Execute(stdout, bufferVars{
				StepName: stepName,
				Buffer:   buffer,
				Error:    err,
				Symbol:   status.Render(),
			})

			if err != nil {
				return err
			}

			return nil
		}
	}
}
