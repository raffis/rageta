package provider

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/fluxcd/pkg/oci"
)

type ociPuller interface {
	Pull(context.Context, string, string, ...oci.PullOption) (*oci.Metadata, error)
}

func WithOCI(ociClient ociPuller) Resolver {
	return func(ctx context.Context, ref string) (io.Reader, error) {
		tmp, err := os.MkdirTemp("", "rageta")
		if err != nil {
			return nil, err
		}
		defer func() {
			_ = os.RemoveAll(tmp)
		}()

		_, err = ociClient.Pull(ctx, ref, tmp)
		if err != nil {
			return nil, err
		}

		return os.Open(filepath.Join(tmp, "main.yaml"))
	}
}
