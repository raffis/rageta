package display

import (
	"fmt"
	"io"
	"os"

	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/stats"
	"github.com/raffis/rageta/internal/styles"
	"github.com/raffis/rageta/internal/xio"
)

func Passthrough(stdout, stderr io.Writer) processor.DisplayFactory {
	gate := &xio.MutexWriter{}
	stdout, stderr = gate.WriterPair(stdout, stderr)

	return func(ctx processor.TaskContext, taskName, short string) processor.Display {
		d := &passthroughDisplay{
			gate:   gate,
			stdout: xio.NewLineWriter(stdout),
		}
		if stdout == stderr {
			d.stderr = d.stdout
		} else {
			d.stderr = xio.NewLineWriter(stderr)
		}

		d.events = xio.NewLineWriter(xio.NewPrefixWriter(xio.NewLipglossWriter(d.stderr, styles.Highlight), []byte("➤ ")))

		return d
	}
}

type passthroughDisplay struct {
	gate                   *xio.MutexWriter
	stdout, stderr, events *xio.LineWriter
}

func (d *passthroughDisplay) Stdout() io.Writer {
	return d.stdout
}

func (d *passthroughDisplay) Stderr() io.Writer {
	return d.stderr
}

func (d *passthroughDisplay) Events() io.Writer {
	return io.Discard
}

func (d *passthroughDisplay) Interrupt(f func(stdin io.Reader, stdout, stderr io.Writer) error) error {
	d.gate.Lock()
	defer d.gate.Unlock()

	return f(os.Stdin, os.Stdout, os.Stderr)
}

func (d *passthroughDisplay) Close(_ processor.TaskContext, _ error) error {
	if err := d.stdout.Flush(); err != nil {
		return fmt.Errorf("error flushing stdout: %w", err)
	}
	if err := d.stderr.Flush(); err != nil {
		return fmt.Errorf("error flushing stderr: %w", err)
	}
	return nil
}

func (d *passthroughDisplay) WriteStats(sample *stats.Sample) error {
	return nil
}

func (d *passthroughDisplay) WritePullProgress(current, total int64) error {
	return nil
}
