package output

import (
	"io"

	"github.com/raffis/rageta/internal/processor"
)

func Discard() processor.OutputFactory {
	return func(ctx processor.StepContext, stepName, short string) (io.Writer, io.Writer, processor.OutputCloser) {
		return io.Discard, io.Discard, func(err error) error {
			return nil
		}
	}
}
