package display

import (
	"io"

	"github.com/raffis/rageta/internal/processor"
)

func Discard() processor.DisplayFactory {
	return func(ctx processor.TaskContext, stepName, short string) (io.Writer, io.Writer, processor.DisplayCloser) {
		return io.Discard, io.Discard, func(_ processor.TaskContext, err error) error {
			return nil
		}
	}
}
