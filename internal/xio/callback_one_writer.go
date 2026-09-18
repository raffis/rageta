package xio

import (
	"io"
	"sync"
)

func NewCallbackOnceWriter(w io.Writer, cb func()) *CallbackOnceWriter {
	return &CallbackOnceWriter{
		w:  w,
		cb: cb,
	}
}

type CallbackOnceWriter struct {
	w    io.Writer
	cb   func()
	once sync.Once
}

func (w *CallbackOnceWriter) Write(p []byte) (int, error) {
	w.once.Do(w.cb)
	_, err := w.w.Write(p)
	return len(p), err
}
