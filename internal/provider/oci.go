package provider

import (
	"context"
	"fmt"
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
			return nil, fmt.Errorf("oci: failed to create temp directory: %w", err)
		}
		defer func() {
			_ = os.RemoveAll(tmp)
		}()

		_, err = ociClient.Pull(ctx, ref, tmp)
		if err != nil {
			return nil, fmt.Errorf("oci: failed to pull image: %w", err)
		}

		r, err := os.Open(filepath.Join(tmp, "main.yaml"))
		if err != nil {
			return nil, fmt.Errorf("oci: failed to open manifest: %w", err)
		}

		return r, nil
	}
}
