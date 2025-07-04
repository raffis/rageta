package provider

import (
	"context"
	"errors"
	"io"
	"os"
)

const RagetaFile = "rageta.yaml"

func WithRagetafile() Resolver {
	return func(ctx context.Context, ref string) (io.Reader, error) {
		if ref != "" {
			return nil, errors.New("no ref expected")
		}

		return os.Open(RagetaFile)
	}
}
