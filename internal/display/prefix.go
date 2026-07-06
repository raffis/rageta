package display

import (
	"fmt"
	"io"

	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/xio"
)

func Prefix(stdout, stderr io.Writer) processor.DisplayFactory {
	return func(ctx processor.TaskContext, stepName, short string) processor.Display {
		prefix := fmt.Appendf(nil, "%s ", ctx.Style.Style.Render(ctx.UniqueName()))
		d := &prefixDisplay{
			stdout: xio.NewLineWriter(xio.NewPrefixWriter(stdout, prefix)),
		}
		if stdout == stderr {
			d.stderr = d.stdout
		} else {
			d.stderr = xio.NewLineWriter(xio.NewPrefixWriter(stderr, prefix))
		}
		return d
	}
}

type prefixDisplay struct {
	stdout, stderr *xio.LineWriter
}

func (d *prefixDisplay) Stdout() io.Writer {
	return d.stdout
}

func (d *prefixDisplay) Stderr() io.Writer {
	return d.stderr
}

func (d *prefixDisplay) Close(_ processor.TaskContext, _ error) error {
	if err := d.stdout.Flush(); err != nil {
		return fmt.Errorf("error flushing stdout: %w", err)
	}
	if err := d.stderr.Flush(); err != nil {
		return fmt.Errorf("error flushing stderr: %w", err)
	}
	return nil
}

func (d *prefixDisplay) WriteStats(cpu, mem, netRx, netTx int64) error {
	d.stderr.Write([]byte(fmt.Sprintf("cpu=%d mem=%d net_rx=%d net_tx=%d\n", cpu, mem, netRx, netTx)))
	return nil
}

func (d *prefixDisplay) WriteProgress(current, total int64) error {
	d.stderr.Write([]byte(fmt.Sprintf("pulling %d/%d bytes\n", current, total)))
	return nil
}
