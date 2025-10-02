package xio

import (
	"bytes"
	"io"
)

func NewSafeWriter(w io.Writer) *SafeWriter {
	return &SafeWriter{
		w:  w,
		ch: make(chan []byte),
	}
}

type SafeWriter struct {
	w  io.Writer
	ch chan []byte
}

func (w *SafeWriter) Write(p []byte) (int, error) {
	w.ch <- bytes.Clone(p)
	return len(p), nil
}

func (w *SafeWriter) SafeWrite() error {
	for msg := range w.ch {
		_, err := w.w.Write(msg)
		if err != nil {
			return err
		}
	}

	return nil
}
