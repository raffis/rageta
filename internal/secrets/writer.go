package secrets

import (
	"bytes"
	"context"
	"io"
)

type maskedWriter struct {
	ctx   context.Context
	mask  []byte
	w     io.Writer
	store Interface
}

func (w *maskedWriter) Write(b []byte) (n int, err error) {
	for _, v := range w.store.Clone(w.ctx) {
		b = bytes.ReplaceAll(b, v, w.mask)
	}

	return w.w.Write(b)
}
