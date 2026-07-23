package checklist

import (
	"io"
	"os"

	"golang.org/x/term"
)

// Display renders steps one at a time.
type Display interface {
	Step(label string, fn func() error) error
	Close() error
}

// New picks a tty or raw Display for out depending on whether it is
// connected to a terminal.
func New(out io.Writer) Display {
	if f, ok := out.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		return newTTY(f)
	}

	return newRaw(out)
}
