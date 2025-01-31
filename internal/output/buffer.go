package output

import (
	"bytes"
	"io"
	"sync"
	"text/template"

	"github.com/raffis/rageta/internal/processor"
)

type bufferVars struct {
	StepName string
	Buffer   *bytes.Buffer
}

func Buffer(tmpl *template.Template) processor.OutputFactory {
	mu := sync.RWMutex{}

	return func(name string, stdin io.Reader, stdout, _ io.Writer) (io.Reader, io.Writer, io.Writer, processor.OutputCloser) {
		buffer := &bytes.Buffer{}

		return stdin, buffer, buffer, func(err error) {
			mu.Lock()
			defer mu.Unlock()

			err = tmpl.Execute(stdout, bufferVars{
				StepName: name,
				Buffer:   buffer,
			})

			if err != nil {
				panic(err)
			}
		}
	}
}
