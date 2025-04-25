package output

import (
	"context"
	"io"

	"github.com/raffis/rageta/internal/processor"
)

func Raw() processor.OutputFactory {
	return func(_ context.Context, stepContext processor.StepContext, stepName string, stdin io.Reader, stdout, stderr io.Writer) (io.Reader, io.Writer, io.Writer, processor.OutputCloser) {
		return stdin, stdout, stderr, func(err error) error {
			return nil
		}
	}
}
