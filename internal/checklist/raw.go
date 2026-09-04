package checklist

import (
	"fmt"
	"io"
)

// rawDisplay writes one line per step with no cursor control, suitable for
// non-terminal output where spinners and carriage returns would just
// produce escape-code garbage.
type rawDisplay struct {
	out io.Writer
}

func newRaw(out io.Writer) *rawDisplay {
	return &rawDisplay{out: out}
}

func (r *rawDisplay) Step(label string, fn func() error) error {
	fmt.Fprintf(r.out, "* %s\n", label)
	err := fn()
	if err != nil {
		fmt.Fprintf(r.out, "  failed: %s: %s\n", label, err)
	} else {
		fmt.Fprintf(r.out, "  done: %s\n", label)
	}

	return err
}

func (r *rawDisplay) Close() error {
	return nil
}
