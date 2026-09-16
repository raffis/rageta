package checklist

import (
	"fmt"
	"io"
	"sync"
)

// rawDisplay writes one line per step with no cursor control, suitable for
// non-terminal output where spinners and carriage returns would just
// produce escape-code garbage.
type rawDisplay struct {
	out    io.Writer
	mu     sync.Mutex
	closed bool
}

func newRaw(out io.Writer) *rawDisplay {
	return &rawDisplay{out: out}
}

func (r *rawDisplay) Step(label string, fn func() error) error {
	if !r.begin(label) {
		return fn()
	}

	err := fn()

	// A failing step hands the error back to the caller, which reports it in
	// full. Stop rendering right here so nothing this display owns can
	// interleave with that report.
	if err != nil {
		_ = r.Close()
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.closed {
		fmt.Fprintf(r.out, "  done: %s\n", label)
	}

	return nil
}

// begin announces the step and reports whether this display is still
// rendering.
func (r *rawDisplay) begin(label string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return false
	}

	fmt.Fprintf(r.out, "* %s\n", label)
	return true
}

func (r *rawDisplay) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	return nil
}
