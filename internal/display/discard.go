package display

import (
	"io"

	"github.com/raffis/rageta/internal/processor"
)

func Discard() processor.DisplayFactory {
	return func(ctx processor.StepContext, stepName, short string) (io.Writer, io.Writer, processor.DisplayCloser) {
		return io.Discard, io.Discard, func(err error) error {
			return nil
		}
	}
}
