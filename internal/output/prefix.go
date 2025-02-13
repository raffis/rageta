package output

import (
	"io"
	"sync"

	"github.com/raffis/rageta/internal/processor"
)

func Prefix(color bool) processor.OutputFactory {
	mu := &sync.Mutex{}

	return func(name string, stdin io.Reader, stdout, stderr io.Writer) (io.Reader, io.Writer, io.Writer, processor.OutputCloser) {
		stdout, stderr = prefixWriter(name, stdout, stderr, color, &lockInfo{
			mu: mu,
		})

		return stdin, stdout, stderr, func(err error) {
			if p, ok := stdout.(*Prefixer); ok {
				p.Close()
			}
			if p, ok := stderr.(*Prefixer); ok {
				p.Close()
			}
		}
	}
}
