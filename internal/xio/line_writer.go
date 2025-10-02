package xio

import (
	"bytes"
	"io"
)

func NewLineWriter(w io.Writer) *LineWriter {
	return &LineWriter{
		w: w,
	}
}

type LineWriter struct {
	w   io.Writer
	buf bytes.Buffer
}

func (w *LineWriter) Write(p []byte) (int, error) {
	total := 0
	for {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			// no newline, just buffer
			w.buf.Write(p)
			total += len(p)
			return total, nil
		}

		w.buf.Write(p[:i+1])
		_, err := w.w.Write(w.buf.Bytes())
		if err != nil {
			return total, err
		}

		total += i + 1
		w.buf.Reset()
		p = p[i+1:]
	}
}

func (w *LineWriter) Flush() error {
	if w.buf.Len() == 0 {
		return nil
	}

	_, err := w.w.Write(w.buf.Bytes())
	if err != nil {
		return err
	}

	w.buf.Reset()
	return nil
}
