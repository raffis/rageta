package report

import (
	"fmt"
	"io"
)

func Markdown(w io.Writer, steps []stepResult) error {
	fmt.Fprintln(w, "| # | Step | Status | Duration | Error |")
	fmt.Fprintln(w, "| --- | --- | --- | --- | --- |")

	for i, step := range steps {
		errMsg, status, duration := stringify(step)
		fmt.Fprintf(w, "| %d | %s | %s | %s | %s |\n",
			i,
			step.stepName,
			status,
			duration,
			errMsg,
		)
	}

	return nil
}
