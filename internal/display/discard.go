package display

import (
	"io"

	"github.com/raffis/rageta/internal/processor"
)

func Discard() processor.DisplayFactory {
	return func(ctx processor.TaskContext, stepName, short string) processor.Display {
		return &discardDisplay{}
	}
}

type discardDisplay struct{}

func (d *discardDisplay) Stdout() io.Writer {
	return io.Discard
}

func (d *discardDisplay) Stderr() io.Writer {
	return io.Discard
}

func (d *discardDisplay) Close(_ processor.TaskContext, _ error) error {
	return nil
}

func (d *discardDisplay) WriteStats(cpu, mem, netRx, netTx int64) error {
	return nil
}
