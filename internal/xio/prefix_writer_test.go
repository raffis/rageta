package xio

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrefixWriter(t *testing.T) {
	tests := []struct {
		name        string
		prefix      []byte
		data        []byte
		expected    []byte
		expectError bool
	}{
		{
			name:        "write with prefix",
			prefix:      []byte("[INFO] "),
			data:        []byte("hello world"),
			expected:    []byte("[INFO] hello world"),
			expectError: false,
		},
		{
			name:        "write with empty prefix",
			prefix:      []byte{},
			data:        []byte("hello world"),
			expected:    []byte("hello world"),
			expectError: false,
		},
		{
			name:        "write with empty data",
			prefix:      []byte("[DEBUG] "),
			data:        []byte{},
			expected:    []byte("[DEBUG] "),
			expectError: false,
		},
		{
			name:        "write with nil data",
			prefix:      []byte("[ERROR] "),
			data:        nil,
			expected:    []byte("[ERROR] "),
			expectError: false,
		},
		{
			name:        "write with nil prefix",
			prefix:      nil,
			data:        []byte("test"),
			expected:    []byte("test"),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			prefixWriter := NewPrefixWriter(&buf, tt.prefix)

			n, err := prefixWriter.Write(tt.data)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				// The Write method returns the length of the input data, not the output
				assert.Equal(t, len(tt.data), n)
				assert.Equal(t, tt.expected, buf.Bytes())
			}
		})
	}
}
