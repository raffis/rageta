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

func TestWithRagetafile(t *testing.T) {
	tests := []struct {
		name        string
		ref         string
		setupFile   func(t *testing.T) string
		expectError bool
		errorMsg    string
	}{
		{
			name: "successful ragetafile read with empty ref",
			ref:  "",
			setupFile: func(t *testing.T) string {
				wd, err := os.Getwd()
				require.NoError(t, err)
				filePath := filepath.Join(wd, RagetaFile)
				err = os.WriteFile(filePath, []byte("test content"), 0644)
				require.NoError(t, err)
				return filePath
			},
			expectError: false,
		},
		{
			name: "error when ref is not empty",
			ref:  "some-ref",
			setupFile: func(t *testing.T) string {
				return ""
			},
			expectError: true,
			errorMsg:    "no ref expected",
		},
		{
			name: "file not found",
			ref:  "",
			setupFile: func(t *testing.T) string {
				// Don't create the file
				return ""
			},
			expectError: true,
			errorMsg:    "no such file or directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up any existing rageta.yaml file
			wd, err := os.Getwd()
			require.NoError(t, err)
			existingFile := filepath.Join(wd, RagetaFile)
			_ = os.Remove(existingFile)

			filePath := tt.setupFile(t)
			if filePath != "" {
				defer func() {
					_ = os.Remove(filePath)
				}()
			}

			resolver := WithRagetafile()
			require.NotNil(t, resolver)

			ctx := context.Background()
			reader, err := resolver(ctx, tt.ref)

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

func TestWithRagetafile_ReadContent(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	filePath := filepath.Join(wd, RagetaFile)
	expectedContent := []byte("test ragetafile content\nwith multiple lines")
	err = os.WriteFile(filePath, expectedContent, 0644)
	require.NoError(t, err)
	defer func() {
		_ = os.Remove(filePath)
	}()

	resolver := WithRagetafile()
	ctx := context.Background()
	reader, err := resolver(ctx, "")

	require.NoError(t, err)
	require.NotNil(t, reader)

	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, expectedContent, data)
}

func TestWithRagetafile_NonEmptyRef(t *testing.T) {
	resolver := WithRagetafile()
	ctx := context.Background()

	testCases := []string{
		"some-ref",
		"file.yaml",
		"path/to/file",
		"oci://example.com/image:tag",
	}

	for _, ref := range testCases {
		t.Run("ref: "+ref, func(t *testing.T) {
			reader, err := resolver(ctx, ref)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "no ref expected")
			assert.Nil(t, reader)
		})
	}
}
