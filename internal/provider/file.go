package provider

import (
	"context"
	"io"
	"os"
)

func WithFile() Resolver {
	return func(ctx context.Context, ref string) (io.Reader, error) {
		return os.Open(ref)
	}
}
