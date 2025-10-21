package xio

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLineWriter(t *testing.T) {
	tests := []struct {
		name        string
		setupWriter func() io.Writer
		data        []byte
		expected    []byte
		expectError bool
	}{
		{
			name: "write single line with newline",
			setupWriter: func() io.Writer {
				return &bytes.Buffer{}
			},
			data:        []byte("hello world\n"),
			expected:    []byte("hello world\n"),
			expectError: false,
		},
		{
			name: "write single line without newline",
			setupWriter: func() io.Writer {
				return &bytes.Buffer{}
			},
			data:        []byte("hello world"),
			expected:    nil, // Should be buffered until newline, so nothing written yet
			expectError: false,
		},
		{
			name: "write multiple lines",
			setupWriter: func() io.Writer {
				return &bytes.Buffer{}
			},
			data:        []byte("line1\nline2\nline3"),
			expected:    []byte("line1\nline2\n"),
			expectError: false,
		},
		{
			name: "write empty data",
			setupWriter: func() io.Writer {
				return &bytes.Buffer{}
			},
			data:        []byte{},
			expected:    nil, // Nothing written
			expectError: false,
		},
		{
			name: "write with only newlines",
			setupWriter: func() io.Writer {
				return &bytes.Buffer{}
			},
			data:        []byte("\n\n\n"),
			expected:    []byte("\n\n\n"),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writer := tt.setupWriter()
			lineWriter := NewLineWriter(writer)

			n, err := lineWriter.Write(tt.data)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, len(tt.data), n)
				if buf, ok := writer.(*bytes.Buffer); ok {
					if tt.expected == nil {
						assert.Empty(t, buf.Bytes())
					} else {
						assert.Equal(t, tt.expected, buf.Bytes())
					}
				}
			}
		})
	}
}

func TestLineWriterFlush(t *testing.T) {
	tests := []struct {
		name        string
		setupWriter func() io.Writer
		data        []byte
		expected    []byte
		expectError bool
	}{
		{
			name: "flush buffered data",
			setupWriter: func() io.Writer {
				return &bytes.Buffer{}
			},
			data:        []byte("buffered data"),
			expected:    []byte("buffered data\n"),
			expectError: false,
		},
		{
			name: "flush empty buffer",
			setupWriter: func() io.Writer {
				return &bytes.Buffer{}
			},
			data:        []byte{},
			expected:    nil, // Nothing to flush
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writer := tt.setupWriter()
			// Create LineWriter directly instead of using NewLineWriter
			lineWriter := &LineWriter{w: writer}

			// Write data without newline (should be buffered)
			_, err := lineWriter.Write(tt.data)
			require.NoError(t, err)

			// Flush the buffer
			err = lineWriter.Flush()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if buf, ok := writer.(*bytes.Buffer); ok {
					if tt.expected == nil {
						assert.Empty(t, buf.Bytes())
					} else {
						assert.Equal(t, tt.expected, buf.Bytes())
					}
				}
			}
		})
	}
}
