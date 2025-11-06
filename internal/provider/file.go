package provider

import (
	"context"
	"fmt"
	"io"
	"os"
)

func WithFile() Resolver {
	return func(ctx context.Context, ref string) (io.Reader, error) {
		r, err := os.Open(ref)
		if err != nil {
			return nil, fmt.Errorf("file: failed to open file: %w", err)
		}

		return r, nil
	}
}
