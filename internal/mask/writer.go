package mask

import (
	"bytes"
	"io"
)

type maskedWriter struct {
	w     io.Writer
	store *SecretStore
}

func (w *maskedWriter) Write(b []byte) (n int, err error) {
	for _, secret := range w.store.secrets {
		b = bytes.ReplaceAll(b, secret, w.store.placeholder)
	}

	return w.w.Write(b)
}
