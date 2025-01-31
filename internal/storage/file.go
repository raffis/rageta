package storage

import (
	"context"
	"io"
	"os"
)

func WithFile() LookupHandler {
	return func(ctx context.Context, ref string) (io.Reader, error) {
		return os.Open(ref)
	}
}
