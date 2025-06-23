package output

import (
	"io"

	"github.com/raffis/rageta/internal/processor"
)

func Passthrough(stdout, stderr io.Writer) processor.OutputFactory {
	return func(ctx processor.StepContext, stepName, short string) (io.Writer, io.Writer, processor.OutputCloser) {
		return stdout, stderr, func(err error) error {
			return nil
		}
	}
}
