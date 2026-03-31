package mask

import (
	"io"
	"sync"
)

var DefaultMask []byte = []byte("***")

func NewSecretStore(mask []byte) *SecretStore {
	return &SecretStore{
		placeholder: DefaultMask,
	}
}

type SecretStore struct {
	mu          sync.Mutex
	placeholder []byte
	secrets     [][]byte
}

func (s *SecretStore) AddSecrets(secrets ...[]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secrets = append(secrets, secrets...)
}

func (s *SecretStore) Writer(w io.Writer) io.Writer {
	return &maskedWriter{
		w:     w,
		store: s,
	}
}
