package provider

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithFile(t *testing.T) {
	tests := []struct {
		name        string
		ref         string
		setupFile   func(t *testing.T) string
		expectError bool
		errorMsg    string
	}{
		{
			name: "successful file read",
			ref:  "",
			setupFile: func(t *testing.T) string {
				tmpDir := t.TempDir()
				filePath := filepath.Join(tmpDir, "test.yaml")
				err := os.WriteFile(filePath, []byte("test content"), 0644)
				require.NoError(t, err)
				return filePath
			},
			expectError: false,
		},
		{
			name: "file not found",
			ref:  "/nonexistent/file.yaml",
			setupFile: func(t *testing.T) string {
				return "/nonexistent/file.yaml"
			},
			expectError: true,
			errorMsg:    "no such file or directory",
		},
		{
			name: "empty ref",
			ref:  "",
			setupFile: func(t *testing.T) string {
				return ""
			},
			expectError: true,
			errorMsg:    "no such file or directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := WithFile()
			require.NotNil(t, resolver)

			filePath := tt.setupFile(t)
			if filePath == "" {
				filePath = tt.ref
			}

			ctx := context.Background()
			reader, err := resolver(ctx, filePath)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				assert.Nil(t, reader)
			} else {
				require.NoError(t, err)
				require.NotNil(t, reader)
				data, err := io.ReadAll(reader)
				require.NoError(t, err)
				assert.NotEmpty(t, data)
			}
		})
	}
}

func TestWithFile_ReadContent(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.yaml")
	expectedContent := []byte("test file content\nwith multiple lines")
	err := os.WriteFile(filePath, expectedContent, 0644)
	require.NoError(t, err)

	resolver := WithFile()
	ctx := context.Background()
	reader, err := resolver(ctx, filePath)

	require.NoError(t, err)
	require.NotNil(t, reader)

	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, expectedContent, data)
}

func TestWithFile_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.yaml")
	err := os.WriteFile(filePath, []byte("test content"), 0644)
	require.NoError(t, err)

	resolver := WithFile()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	reader, err := resolver(ctx, filePath)

	// os.Open doesn't respect context cancellation, so this should still succeed
	// but we test that the resolver works with a cancelled context
	if err == nil {
		require.NotNil(t, reader)
		if closer, ok := reader.(io.Closer); ok {
			_ = closer.Close()
		}
	}
}
