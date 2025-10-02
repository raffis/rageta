package xio

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSafeWriter(t *testing.T) {
	tests := []struct {
		name           string
		setupWriter    func() io.Writer
		data           []byte
		expectWriteErr bool
		expectSafeErr  bool
	}{
		{
			name: "successful safe write",
			setupWriter: func() io.Writer {
				return &bytes.Buffer{}
			},
			data:           []byte("test data"),
			expectWriteErr: false,
			expectSafeErr:  false,
		},
		{
			name: "multiple writes",
			setupWriter: func() io.Writer {
				return &bytes.Buffer{}
			},
			data:           []byte("multiple writes test"),
			expectWriteErr: false,
			expectSafeErr:  false,
		},
		{
			name: "write with nil data",
			setupWriter: func() io.Writer {
				return &bytes.Buffer{}
			},
			data:           nil,
			expectWriteErr: false,
			expectSafeErr:  false,
		},
		{
			name: "write with empty data",
			setupWriter: func() io.Writer {
				return &bytes.Buffer{}
			},
			data:           nil,
			expectWriteErr: false,
			expectSafeErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writer := tt.setupWriter()
			safeWriter := NewSafeWriter(writer)

			// Test SafeWrite method - this will block until we close the channel
			// We need to run this in a goroutine and close the channel
			done := make(chan error)
			go func() {
				done <- safeWriter.SafeWrite()
			}()

			// Test Write method
			n, err := safeWriter.Write(tt.data)
			if tt.expectWriteErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, len(tt.data), n)
			}

			// Close the channel to unblock SafeWrite
			close(safeWriter.ch)

			// Wait for SafeWrite to complete
			err = <-done
			if tt.expectSafeErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				// Verify data was written to the underlying writer
				if buf, ok := writer.(*bytes.Buffer); ok {
					assert.Equal(t, tt.data, buf.Bytes())
				}
			}
		})
	}
}
