package provider

import (
	"context"
	"errors"
	"io"
	"os"
)

func WithRagetafile() LookupHandler {
	return func(ctx context.Context, ref string) (io.Reader, error) {
		if ref != "" {
			return nil, errors.New("no ref expected")
		}

		return os.Open("rageta.yaml")
	}
}
