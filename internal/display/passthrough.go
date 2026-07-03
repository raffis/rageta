package display

import (
	"fmt"
	"io"

	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/xio"
)

func Passthrough(stdout, stderr io.Writer) processor.DisplayFactory {
	return func(ctx processor.TaskContext, stepName, short string) processor.Display {
		d := &passthroughDisplay{
			stdout: xio.NewLineWriter(stdout),
		}
		if stdout == stderr {
			d.stderr = d.stdout
		} else {
			d.stderr = xio.NewLineWriter(stderr)
		}
		return d
	}
}

type passthroughDisplay struct {
	stdout, stderr *xio.LineWriter
}

func (d *passthroughDisplay) Stdout() io.Writer {
	return d.stdout
}

func (d *passthroughDisplay) Stderr() io.Writer {
	return d.stderr
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

func (d *passthroughDisplay) WriteStats(cpu, mem, netRx, netTx int64) error {
	return nil
}
