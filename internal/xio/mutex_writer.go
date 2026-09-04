package xio

import (
	"io"
	"sync"
)

type MutexWriter struct {
	mu sync.RWMutex
}

// Writer wraps w so every write is serialized against an exclusive Lock.
func (g *MutexWriter) Writer(w io.Writer) io.Writer {
	return &mutexWriter{gate: g, w: w}
}

// Lock excludes every writer produced by Writer until Unlock is called.
func (g *MutexWriter) Lock() { g.mu.Lock() }

// Unlock releases a prior Lock, letting blocked writers proceed.
func (g *MutexWriter) Unlock() { g.mu.Unlock() }

// WriterPair wraps a and b, preserving their identity relationship: if a and
// b were the same writer to begin with (e.g. stdout and stderr combined into
// one stream), the two returned writers are the same value too, so callers
// that branch on that (== comparisons, to avoid writing everything twice)
// keep working after wrapping.
func (g *MutexWriter) WriterPair(a, b io.Writer) (io.Writer, io.Writer) {
	wa := g.Writer(a)
	if a == b {
		return wa, wa
	}

	return wa, g.Writer(b)
}

type mutexWriter struct {
	gate *MutexWriter
	w    io.Writer
}

func (g *mutexWriter) Write(p []byte) (int, error) {
	g.gate.mu.RLock()
	defer g.gate.mu.RUnlock()
	return g.w.Write(p)
}
