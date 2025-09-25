package xio

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockFDCapable struct {
	readData []byte
	readErr  error
	closeErr error
	fd       uintptr
}

func (m *mockFDCapable) Read(p []byte) (n int, err error) {
	if m.readErr != nil {
		return 0, m.readErr
	}
	if len(m.readData) == 0 {
		return 0, io.EOF
	}
	n = copy(p, m.readData)
	m.readData = m.readData[n:]
	return n, nil
}

func (m *mockFDCapable) Close() error {
	return m.closeErr
}

func (m *mockFDCapable) Fd() uintptr {
	return m.fd
}

func TestFDWrapper(t *testing.T) {
	t.Run("basic functionality", func(t *testing.T) {
		// Setup
		readData := []byte("hello world")
		fdCapable := &mockFDCapable{
			readData: make([]byte, len(readData)),
			fd:       123,
		}
		copy(fdCapable.readData, readData)

		writer := &bytes.Buffer{}
		wrapper := NewFDWrapper(writer, fdCapable)

		// Test Read
		buffer := make([]byte, 20)
		n, err := wrapper.Read(buffer)
		assert.NoError(t, err)
		assert.Equal(t, len(readData), n)
		assert.Equal(t, readData, buffer[:n])

		// Test Write
		n, err = wrapper.Write(buffer[:n])
		assert.NoError(t, err)
		assert.Equal(t, len(readData), n)
		assert.Equal(t, readData, writer.Bytes())

		// Test Fd
		fd := wrapper.Fd()
		assert.Equal(t, uintptr(123), fd)

		// Test Close
		err = wrapper.Close()
		assert.NoError(t, err)
	})

	t.Run("error handling", func(t *testing.T) {
		// Test read error
		fdCapable := &mockFDCapable{
			readErr: errors.New("read error"),
			fd:      456,
		}
		wrapper := NewFDWrapper(&bytes.Buffer{}, fdCapable)

		buffer := make([]byte, 10)
		_, err := wrapper.Read(buffer)
		assert.Error(t, err)
		assert.Equal(t, "read error", err.Error())

		// Test close error
		fdCapable.closeErr = errors.New("close error")
		err = wrapper.Close()
		assert.Error(t, err)
		assert.Equal(t, "close error", err.Error())
	})

	t.Run("interface compliance", func(t *testing.T) {
		var _ io.Reader = (*FDWrapper)(nil)
		var _ io.Writer = (*FDWrapper)(nil)
		var _ io.Closer = (*FDWrapper)(nil)
	})
}
