package display

import (
	"fmt"
	"io"

	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/xio"
)

func Passthrough(stdout, stderr io.Writer) processor.DisplayFactory {
	return func(ctx processor.TaskContext, stepName, short string) (io.Writer, io.Writer, processor.DisplayCloser) {
		stdoutWrapper := xio.NewLineWriter(stdout)
		stderrWrapper := stdoutWrapper

		if stdout != stderr {
			stderrWrapper = xio.NewLineWriter(stderr)
		}

		return stdoutWrapper, stderrWrapper, func(err error) error {
			if err := stdoutWrapper.Flush(); err != nil {
				return fmt.Errorf("error flushing stdout: %w", err)
			}
			if err := stderrWrapper.Flush(); err != nil {
				return fmt.Errorf("error flushing stderr: %w", err)
			}

			return nil
		}
	}
}
