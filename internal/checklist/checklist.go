// Package checklist renders a sequential terminal checklist, e.g.:
//
//	[x] Connecting to docker
//	[x] Creating buildkit container
//	⠋ Waiting for buildkit
package checklist

import (
	"fmt"
	"io"
	"time"
)

var frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Checklist renders steps one at a time to out. A nil *Checklist is valid and
// runs steps without any output, so callers can pass one through optionally.
type Checklist struct {
	out io.Writer
}

func New(out io.Writer) *Checklist {
	return &Checklist{out: out}
}

// Step renders label with a spinner while fn runs, then finalizes the line
// with a checkmark on success or a cross on failure.
func (c *Checklist) Step(label string, fn func() error) error {
	if c == nil {
		return fn()
	}

	done := make(chan error, 1)
	go func() {
		done <- fn()
	}()

	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	i := 0
	fmt.Fprintf(c.out, "%s %s", frames[i], label)

	for {
		select {
		case err := <-done:
			fmt.Fprint(c.out, "\r\033[K")
			if err != nil {
				fmt.Fprintf(c.out, "✗ %s\n", label)
			} else {
				fmt.Fprintf(c.out, "[x] %s\n", label)
			}
			return err
		case <-ticker.C:
			i++
			fmt.Fprintf(c.out, "\r\033[K%s %s", frames[i%len(frames)], label)
		}
	}
}
