package xio

import (
	"io"
)

func NewPrefixWriter(w io.Writer, prefix []byte) *PrefixWriter {
	return &PrefixWriter{
		w:      w,
		prefix: prefix,
	}
}

type PrefixWriter struct {
	w      io.Writer
	prefix []byte
}

func (w *PrefixWriter) Write(p []byte) (int, error) {
	_, err := w.w.Write(append(w.prefix, p...))
	return len(p), err
}
