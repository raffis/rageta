package output

import (
	"bytes"
	"io"
	"sync"
	"text/template"

	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/tui"
)

type bufferVars struct {
	StepName    string
	DisplayName string
	UniqueName  string
	Buffer      *bytes.Buffer
	Error       error
	Symbol      string
}

func Buffer(tmpl *template.Template, stdout io.Writer) processor.OutputFactory {
	mu := sync.RWMutex{}

	return func(ctx processor.StepContext, stepName, short string) (io.Writer, io.Writer, processor.OutputCloser) {
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

			displayName := stepName
			if short != "" {
				displayName = short
			}

			err = tmpl.Execute(stdout, bufferVars{
				StepName:    stepName,
				UniqueName:  processor.SuffixName(stepName, ctx.NamePrefix),
				DisplayName: displayName,
				Buffer:      buffer,
				Error:       err,
				Symbol:      status.Render(),
			})

			if err != nil {
				return err
			}

			return nil
		}
	}
}
