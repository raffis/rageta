package output

import (
	"fmt"
	"io"

	"github.com/charmbracelet/lipgloss"
	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/styles"
	"github.com/raffis/rageta/internal/xio"
)

func Prefix(stdout, stderr io.Writer) processor.OutputFactory {
	return func(ctx processor.StepContext, stepName, short string) (io.Writer, io.Writer, processor.OutputCloser) {
		uniqueName := processor.SuffixName(stepName, ctx.NamePrefix)
		style := lipgloss.NewStyle().Foreground(styles.RandAdaptiveColor())

		stdoutWrapper := xio.NewLineWriter(xio.NewPrefixWriter(stdout, fmt.Appendf(nil, "%s ", style.Render(uniqueName))))
		stderrWrapper := stdoutWrapper

		if stdout != stderr {
			stderrWrapper = xio.NewLineWriter(xio.NewPrefixWriter(stderr, fmt.Appendf(nil, "%s ", style.Render(uniqueName))))
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
