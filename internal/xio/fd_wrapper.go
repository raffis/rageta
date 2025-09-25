package xio

import (
	"io"
)

func NewFDWrapper(w io.Writer, f fdCapable) *FDWrapper {
	return &FDWrapper{
		w: w,
		f: f,
	}
}

type fdCapable interface {
	io.Reader
	io.Closer
	Fd() uintptr
}

type FDWrapper struct {
	w io.Writer
	f fdCapable
}

func (f *FDWrapper) Read(p []byte) (n int, err error) {
	return f.f.Read(p)
}

func (f *FDWrapper) Write(p []byte) (n int, err error) {
	return f.w.Write(p)
}

func (f *FDWrapper) Close() error {
	return f.f.Close()
}

func (f *FDWrapper) Fd() uintptr {
	return f.f.Fd()
}
