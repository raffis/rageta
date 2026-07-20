package display

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"text/template"

	"github.com/raffis/rageta/internal/processor"
)

func Buffer(tmpl *template.Template, dev io.Writer) processor.DisplayFactory {
	mu := sync.RWMutex{}

	return func(ctx processor.TaskContext, stepName, short string) processor.Display {
		return &bufferDisplay{
			buf:   &bytes.Buffer{},
			mu:    &mu,
			tmpl:  tmpl,
			dev:   dev,
			ctx:   ctx,
			name:  stepName,
			short: short,
		}
	}
}

type bufferVars struct {
	TaskName    string
	DisplayName string
	UniqueName  string
	Buffer      string
	Error       error
	Skipped     bool
	Labels      []processor.Label
}

type bufferDisplay struct {
	buf   *bytes.Buffer
	mu    *sync.RWMutex
	tmpl  *template.Template
	dev   io.Writer
	ctx   processor.TaskContext
	name  string
	short string
}

func (d *bufferDisplay) Stdout() io.Writer {
	return d.buf
}

func (d *bufferDisplay) Stderr() io.Writer {
	return d.buf
}

func (d *bufferDisplay) Dev() io.Writer {
	return io.Discard
}

func (d *bufferDisplay) Start(_ processor.TaskContext) error {
	return nil
}

func (d *bufferDisplay) Close(_ processor.TaskContext, err error) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	displayName := d.name
	if d.short != "" {
		displayName = d.short
	}

	return d.tmpl.Execute(d.dev, bufferVars{
		TaskName:    d.name,
		UniqueName:  d.ctx.UniqueName(),
		DisplayName: displayName,
		Buffer:      strings.TrimRight(d.buf.String(), "\n"),
		Error:       err,
		Skipped:     err != nil && !processor.AbortOnError(err),
		Labels:      d.ctx.Labels.Labels(),
	})
}

func (d *bufferDisplay) WriteStats(cpu, mem, netRx, netTx int64) error {
	return nil
}

func (d *bufferDisplay) WriteProgress(current, total int64) error {
	return nil
}
