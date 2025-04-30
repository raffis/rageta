package output

import (
	"bytes"
	"context"
	"io"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/raffis/rageta/internal/processor"
)

func writeBytes(w io.Writer, r chan prefixMessage) {
	/*var lastChar byte
	var lastProducer string
	*/
	id := bytes.Fields(debug.Stack())[1]
	for msg := range r {
		/*if lastProducer != "" && lastProducer != msg.producer && lastChar != byte('\n') {
			w.Write([]byte{'\n'})
		}*

		//lastChar = msg.b[len(msg.b)-1]
		//lastProducer = msg.producer



		//fmt.Printf("LINE: %#v -- %#v\n\n", string(msg.b), lines)
		*/
		lines := strings.Split(strings.ReplaceAll(strings.TrimSuffix(string(msg.b), "\n"), "\r", ""), "\n")
		for _, line := range lines {

			w.Write([]byte(msg.style.Render(msg.producer)))
			w.Write(id)
			w.Write([]byte(line))

			w.Write([]byte{'\n'})

		}

		//w.Write(msg.b)
	}
}

type prefixMessage struct {
	style    lipgloss.Style
	producer string
	b        []byte
}

func Prefix(color bool) processor.OutputFactory {
	mu := sync.Mutex{}
	writers := make(map[io.Writer]chan prefixMessage)
	var count int32

	return func(_ context.Context, stepContext processor.StepContext, stepName string, stdin io.Reader, stdout, stderr io.Writer) (io.Reader, io.Writer, io.Writer, processor.OutputCloser) {
		var stdoutCh chan prefixMessage
		var stderrCh chan prefixMessage

		mu.Lock()
		defer mu.Unlock()
		count++

		if w, ok := writers[stdout]; ok {
			stdoutCh = w
		} else {
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

		stdoutWrapper, stderrWrapper := prefixWriter(stepName, stdoutCh, stderrCh, color)

		return stdin, stdoutWrapper, stderrWrapper, func(err error) error {
			mu.Lock()
			defer mu.Unlock()

			count--
			if count == 0 {
				close(writers[stdout])
				close(writers[stderr])
				delete(writers, stdout)
				delete(writers, stderr)
			}

			return nil
		}
	}
}
