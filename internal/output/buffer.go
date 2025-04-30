package output

import (
	"bytes"
	"context"
	"io"
	"sync"
	"text/template"

	"github.com/raffis/rageta/internal/processor"
)

type bufferVars struct {
	StepName string
	Buffer   *bytes.Buffer
}

func Buffer(tmpl *template.Template, stdout io.Writer) processor.OutputFactory {
	mu := sync.RWMutex{}

	return func(_ context.Context, stepContext processor.StepContext, stepName string) (io.Writer, io.Writer, processor.OutputCloser) {
		buffer := &bytes.Buffer{}

		return buffer, buffer, func(err error) error {
			mu.Lock()
			defer mu.Unlock()

			err = tmpl.Execute(stdout, bufferVars{
				StepName: stepContext.NamePrefix,
				Buffer:   buffer,
			})

			if err != nil {
				return err
			}

			return nil
		}
	}
}
