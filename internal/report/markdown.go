package report

import (
	"fmt"
	"io"
)

func Markdown(w io.Writer, store *Store) error {
	fmt.Fprintln(w, "| # | Step | Status | Duration | Error |")
	fmt.Fprintln(w, "| --- | --- | --- | --- | --- |")

	for i, step := range store.steps {
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
