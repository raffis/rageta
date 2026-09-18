package secrets

import (
	"context"
	"errors"
	"io"
)

var ErrNotFound = errors.New("secret not found")

type Interface interface {
	GetSecret(ctx context.Context, key string) ([]byte, error)
	AddSecret(ctx context.Context, key string, value []byte)
	Clone(ctx context.Context) map[string][]byte
	Pipe(ctx context.Context, w io.Writer, mask []byte) io.Writer
}
