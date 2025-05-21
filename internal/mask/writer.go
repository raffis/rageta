package mask

import (
	"bytes"
	"io"
	"sync"

	"github.com/charmbracelet/x/term"
)

type fd interface {
	Fd() uintptr
}

var _ term.File = &maskedWriter{}

func Writer(w io.ReadWriteCloser, maskPlaceholder []byte, secrets ...[]byte) *maskedWriter {
	return &maskedWriter{
		w:           w,
		secrets:     secrets,
		placeholder: maskPlaceholder,
	}
}

type maskedWriter struct {
	w           io.ReadWriteCloser
	mu          sync.Mutex
	placeholder []byte
	secrets     [][]byte
}

func (w *maskedWriter) AddSecrets(secrets ...[]byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.secrets = append(w.secrets, secrets...)
}

func (w *maskedWriter) Write(b []byte) (n int, err error) {
	for _, secret := range w.secrets {
		b = bytes.ReplaceAll(b, secret, w.placeholder)
	}

	return w.w.Write(b)
}

func (w *maskedWriter) Read(p []byte) (n int, err error) {
	return w.w.Read(p)
}

func (w *maskedWriter) Close() error {
	return w.w.Close()
}

func (w *maskedWriter) Fd() uintptr {
	if f, ok := w.w.(fd); ok {
		return f.Fd()
	}

	return 0
}

type SecretStore interface {
	AddSecrets(b ...[]byte)
}

type secretWriter struct {
	m []SecretStore
}

func SecretWriter(m ...SecretStore) *secretWriter {
	return &secretWriter{
		m: m,
	}
}

func (s *secretWriter) AddSecrets(secrets ...[]byte) {
	for _, mask := range s.m {
		mask.AddSecrets(secrets...)
	}
}
