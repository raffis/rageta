package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
)

const RagetaFile = "rageta.yaml"

func WithRagetafile() Resolver {
	return func(ctx context.Context, ref string) (io.Reader, error) {
		if ref != "" {
			return nil, errors.New("ragetafile: no ref expected")
		}

		r, err := os.Open(RagetaFile)
		if err != nil {
			return nil, fmt.Errorf("ragetafile: failed to open file: %w", err)
		}

		return r, nil
	}
}
