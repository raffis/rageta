package secrets

import (
	"context"
	"io"
	"sync"

	"github.com/moby/buildkit/session/secrets"
)

func InMemoryStore() *inMemoryStore {
	return &inMemoryStore{
		store: make(map[string][]byte),
	}
}

type inMemoryStore struct {
	mu    sync.RWMutex
	store map[string][]byte
}

func (s *inMemoryStore) Pipe(ctx context.Context, w io.Writer, mask []byte) io.Writer {
	return &maskedWriter{
		w:     w,
		store: s,
		mask:  mask,
	}
}

func (s *inMemoryStore) GetSecret(_ context.Context, key string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.store[key]

	if !ok {
		return nil, secrets.ErrNotFound
	}
	return v, nil
}

func (s *inMemoryStore) Clone(_ context.Context) map[string][]byte {
	s.mu.RLock()
	defer s.mu.RUnlock()

	copy := make(map[string][]byte, len(s.store))
	for k, v := range s.store {
		copy[k] = v
	}

	return copy
}

func (s *inMemoryStore) AddSecret(_ context.Context, key string, value []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store[key] = value
}
