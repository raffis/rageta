package provider

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/fluxcd/pkg/oci"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockOCIPuller is a mock implementation of ociPuller
type mockOCIPuller struct {
	pullFunc func(ctx context.Context, ref string, path string, opts ...oci.PullOption) (*oci.Metadata, error)
}

func (m *mockOCIPuller) Pull(ctx context.Context, ref string, path string, opts ...oci.PullOption) (*oci.Metadata, error) {
	if m.pullFunc != nil {
		return m.pullFunc(ctx, ref, path, opts...)
	}
	return nil, nil
}

func TestWithOCI(t *testing.T) {
	tests := []struct {
		name            string
		ref             string
		ociClient       ociPuller
		expectError     bool
		errorMsg        string
		expectedContent []byte
		verifyContent   bool
	}{
		{
			name: "successful OCI pull",
			ref:  "oci://example.com/image:tag",
			ociClient: &mockOCIPuller{
				pullFunc: func(ctx context.Context, ref string, path string, opts ...oci.PullOption) (*oci.Metadata, error) {
					filePath := filepath.Join(path, "main.yaml")
					err := os.WriteFile(filePath, []byte("test manifest"), 0644)
					if err != nil {
						return nil, err
					}
					return &oci.Metadata{}, nil
				},
			},
			expectError:     false,
			expectedContent: []byte("test manifest"),
			verifyContent:   true,
		},
		{
			name: "successful OCI pull with multi-line content",
			ref:  "oci://example.com/image:tag",
			ociClient: &mockOCIPuller{
				pullFunc: func(ctx context.Context, ref string, path string, opts ...oci.PullOption) (*oci.Metadata, error) {
					filePath := filepath.Join(path, "main.yaml")
					content := []byte("test manifest content\nwith multiple lines")
					err := os.WriteFile(filePath, content, 0644)
					if err != nil {
						return nil, err
					}
					return &oci.Metadata{}, nil
				},
			},
			expectError:     false,
			expectedContent: []byte("test manifest content\nwith multiple lines"),
			verifyContent:   true,
		},
		{
			name: "OCI pull error",
			ref:  "oci://example.com/image:tag",
			ociClient: &mockOCIPuller{
				pullFunc: func(ctx context.Context, ref string, path string, opts ...oci.PullOption) (*oci.Metadata, error) {
					return nil, assert.AnError
				},
			},
			expectError: true,
		},
		{
			name: "main.yaml missing after pull",
			ref:  "oci://example.com/image:tag",
			ociClient: &mockOCIPuller{
				pullFunc: func(ctx context.Context, ref string, path string, opts ...oci.PullOption) (*oci.Metadata, error) {
					// Don't create main.yaml - simulate missing file
					return &oci.Metadata{}, nil
				},
			},
			expectError: true,
			errorMsg:    "no such file or directory",
		},
		{
			name: "context cancellation in pull",
			ref:  "oci://example.com/image:tag",
			ociClient: &mockOCIPuller{
				pullFunc: func(ctx context.Context, ref string, path string, opts ...oci.PullOption) (*oci.Metadata, error) {
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					default:
						filePath := filepath.Join(path, "main.yaml")
						err := os.WriteFile(filePath, []byte("test"), 0644)
						if err != nil {
							return nil, err
						}
						return &oci.Metadata{}, nil
					}
				},
			},
			expectError: true,
			errorMsg:    "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := WithOCI(tt.ociClient)
			require.NotNil(t, resolver)

			var ctx context.Context
			if tt.name == "context cancellation in pull" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(context.Background())
				cancel() // Cancel immediately
			} else {
				ctx = context.Background()
			}

			reader, err := resolver(ctx, tt.ref)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				if reader != nil {
					if closer, ok := reader.(io.Closer); ok {
						_ = closer.Close()
					}
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, reader)

				if tt.verifyContent {
					data, err := io.ReadAll(reader)
					require.NoError(t, err)
					assert.Equal(t, tt.expectedContent, data)
				} else {
					// Just verify reader is valid
					_, err := io.ReadAll(reader)
					require.NoError(t, err)
				}

				if closer, ok := reader.(io.Closer); ok {
					_ = closer.Close()
				}
			}
		})
	}
}
