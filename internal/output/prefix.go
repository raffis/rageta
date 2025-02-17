package output

import (
	"io"
	"sync"

	"github.com/raffis/rageta/internal/processor"
)

func writeBytes(w io.Writer, r chan prefixMessage) {
	var lastChar byte
	var lastProducer string

	for msg := range r {
		//fmt.Printf("\n%#v -- %#v -- %#v“\n", lastProducer, msg.producer, lastProducer == msg.producer)

		if lastProducer != "" && lastProducer != msg.producer && lastChar != byte('\n') {
			//	w.Write([]byte{'\n'})
		}

		//lastChar = msg.b[len(msg.b)-1]
		lastProducer = msg.producer
		w.Write(msg.b)
	}
}

type prefixMessage struct {
	producer string
	b        []byte
}

func Prefix(color bool) processor.OutputFactory {
	mu := sync.Mutex{}
	writers := make(map[io.Writer]chan prefixMessage)
	var count int32

	return func(name string, stdin io.Reader, stdout, stderr io.Writer) (io.Reader, io.Writer, io.Writer, processor.OutputCloser) {
		var stdoutCh chan prefixMessage
		var stderrCh chan prefixMessage

		mu.Lock()
		defer mu.Unlock()
		count++

		if w, ok := writers[stdout]; ok {
			stdoutCh = w
		} else {
			//fmt.Printf("\n\n\n==================================================== %#v\n\n\n", stdout)
			writers[stdout] = make(chan prefixMessage)
			stdoutCh = writers[stdout]
			go writeBytes(stdout, writers[stdout])
		}
		if w, ok := writers[stderr]; ok {
			stderrCh = w
		} else {
			writers[stderr] = make(chan prefixMessage)
			stderrCh = writers[stderr]
			go writeBytes(stderr, writers[stderr])
		}

		stdoutWrapper, stderrWrapper := prefixWriter(name, stdoutCh, stderrCh, color)

		return stdin, stdoutWrapper, stderrWrapper, func(err error) {
			mu.Lock()
			defer mu.Unlock()

			count--
			if count == 0 {
				close(writers[stdout])
				close(writers[stderr])
				delete(writers, stdout)
				delete(writers, stderr)
			}
		}
	}
}
