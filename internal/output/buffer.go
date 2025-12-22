package output

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"text/template"

	"github.com/raffis/rageta/internal/processor"
)

type bufferVars struct {
	StepName    string
	DisplayName string
	UniqueName  string
	Buffer      string
	Error       error
	Skipped     bool
	Tags        []processor.Tag
}

func Buffer(tmpl *template.Template, dev io.Writer) processor.OutputFactory {
	mu := sync.RWMutex{}

	return func(ctx processor.StepContext, stepName, short string) (io.Writer, io.Writer, processor.OutputCloser) {
		buffer := &bytes.Buffer{}

		return buffer, buffer, func(err error) error {
			mu.Lock()
			defer mu.Unlock()

			displayName := stepName
			if short != "" {
				displayName = short
			}

			err = tmpl.Execute(dev, bufferVars{
				StepName:    stepName,
				UniqueName:  processor.SuffixName(stepName, ctx.NamePrefix),
				DisplayName: displayName,
				Buffer:      strings.TrimRight(buffer.String(), "\n"),
				Error:       err,
				Skipped:     err != nil && !processor.AbortOnError(err),
				Tags:        ctx.Tags(),
			})

			return err
		}
	}
}
