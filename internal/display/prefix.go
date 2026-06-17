package display

import (
	"fmt"
	"io"

	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/xio"
)

func Prefix(stdout, stderr io.Writer) processor.DisplayFactory {
	return func(ctx processor.StepContext, stepName, short string) (io.Writer, io.Writer, processor.DisplayCloser) {
		stdoutWrapper := xio.NewLineWriter(xio.NewPrefixWriter(stdout, fmt.Appendf(nil, "%s ", ctx.Style.Style.Render(ctx.UniqueName()))))
		stderrWrapper := stdoutWrapper

		if stdout != stderr {
			stderrWrapper = xio.NewLineWriter(xio.NewPrefixWriter(stderr, fmt.Appendf(nil, "%s ", ctx.Style.Style.Render(ctx.UniqueName()))))
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
