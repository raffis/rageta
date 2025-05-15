package mask

import (
	"bytes"
	"io"
	"sync"
)

func Writer(w io.Writer, maskPlaceholder []byte, secrets ...[]byte) *maskedWriter {
	return &maskedWriter{
		w:           w,
		secrets:     secrets,
		placeholder: maskPlaceholder,
	}
}

type maskedWriter struct {
	w           io.Writer
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
