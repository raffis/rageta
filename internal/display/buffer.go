package display

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"text/template"

	"github.com/raffis/rageta/internal/processor"
)

type bufferVars struct {
	TaskName    string
	DisplayName string
	UniqueName  string
	Buffer      string
	Error       error
	Skipped     bool
	Labels      []processor.Label
}

func Buffer(tmpl *template.Template, dev io.Writer) processor.DisplayFactory {
	mu := sync.RWMutex{}

	return func(ctx processor.TaskContext, stepName, short string) (io.Writer, io.Writer, processor.DisplayCloser) {
		buffer := &bytes.Buffer{}

		return buffer, buffer, func(err error) error {
			mu.Lock()
			defer mu.Unlock()

			displayName := stepName
			if short != "" {
				displayName = short
			}

			err = tmpl.Execute(dev, bufferVars{
				TaskName:    stepName,
				UniqueName:  ctx.UniqueName(),
				DisplayName: displayName,
				Buffer:      strings.TrimRight(buffer.String(), "\n"),
				Error:       err,
				Skipped:     err != nil && !processor.AbortOnError(err),
				Labels:      ctx.Labels.Labels(),
			})

			return err
		}
	}
}
