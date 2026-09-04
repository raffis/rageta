package xio

import "io"

type NopWriteCloser struct {
	io.Writer
}

func (NopWriteCloser) Close() error { return nil }
