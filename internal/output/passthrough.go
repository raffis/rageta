package output

import (
	"io"

	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/xio"
)

func Passthrough(stdout, stderr io.Writer) processor.OutputFactory {
	return func(ctx processor.StepContext, stepName, short string) (io.Writer, io.Writer, processor.OutputCloser) {
		stdoutWrapper := xio.NewLineWriter(stdout)
		stderrWrapper := xio.NewLineWriter(stderr)

		return stdoutWrapper, stderrWrapper, func(err error) error {
			return nil
		}
	}
}
