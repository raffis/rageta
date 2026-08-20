package display

import (
	"io"

	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/stats"
)

func Discard() processor.DisplayFactory {
	return func(ctx processor.TaskContext, taskName, short string) processor.Display {
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

func (d *discardDisplay) Events() io.Writer {
	return io.Discard
}

func (d *discardDisplay) Close(_ processor.TaskContext, _ error) error {
	return nil
}

func (d *discardDisplay) WriteStats(sample *stats.Sample) error {
	return nil
}

func (d *discardDisplay) WritePullProgress(current, total int64) error {
	return nil
}
