package output

import (
	"context"
	"io"

	"github.com/raffis/rageta/internal/processor"
)

func Passthrough(stdout, stderr io.Writer) processor.OutputFactory {
	return func(_ context.Context, stepContext processor.StepContext, stepName string) (io.Writer, io.Writer, processor.OutputCloser) {
		return stdout, stderr, func(err error) error {
			return nil
		}
	}
}
