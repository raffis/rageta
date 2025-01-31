package output

import (
	"io"

	"github.com/raffis/rageta/internal/processor"
)

func Prefix(color bool) processor.OutputFactory {
	return func(name string, stdin io.Reader, stdout, stderr io.Writer) (io.Reader, io.Writer, io.Writer, processor.OutputCloser) {
		stdout, stderr = prefixWriter(name, stdout, stderr, color)
		return stdin, stdout, stderr, func(err error) {}
	}
}
