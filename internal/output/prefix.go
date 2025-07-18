package output

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/styles"
)

// TerminalWriter writes messages from multiple producers properly prefixed by the message prefix
func TerminalWriter(ch chan PrefixMessage) {
	var lastProducer string
	var newLine bool
	for msg := range ch {
		lines := strings.Split(strings.ReplaceAll(strings.TrimSuffix(string(msg.b), "\n"), "\r", ""), "\n")

		if lastProducer != "" && msg.producer != lastProducer && !newLine {
			_, _ = msg.w.Write([]byte{'\n'})
		}

		for i, line := range lines {
			if i == 0 {
				if (msg.producer == lastProducer && newLine) || msg.producer != lastProducer {
					_, _ = msg.w.Write([]byte(msg.style.Render(msg.producer)))
				}
			} else {
				_, _ = msg.w.Write([]byte(msg.style.Render(msg.producer)))
			}

			_, _ = msg.w.Write([]byte(line))

			if i < len(lines)-1 {
				_, _ = msg.w.Write([]byte{'\n'})
			}
		}

		newLine = strings.HasSuffix(string(msg.b), "\n")
		if newLine {
			_, _ = msg.w.Write([]byte{'\n'})
		}

		lastProducer = msg.producer
	}
}

func Prefix(stdout, stderr io.Writer, ch chan PrefixMessage) processor.OutputFactory {
	return func(ctx processor.StepContext, stepName, short string) (io.Writer, io.Writer, processor.OutputCloser) {
		uniqueName := processor.SuffixName(stepName, ctx.NamePrefix)
		style := lipgloss.NewStyle().Foreground(styles.RandAdaptiveColor())

		stdoutWrapper := &prefixWrapper{
			prefix: fmt.Sprintf("%s ", uniqueName),
			style:  style,
			ch:     ch,
			w:      stdout,
		}

		stderrWrapper := &prefixWrapper{
			prefix: fmt.Sprintf("%s ", uniqueName),
			style:  style,
			ch:     ch,
			w:      stderr,
		}

		return stdoutWrapper, stderrWrapper, func(err error) error {
			return nil
		}
	}
}

type PrefixMessage struct {
	style    lipgloss.Style
	producer string
	b        []byte
	w        io.Writer
}

type prefixWrapper struct {
	prefix string
	style  lipgloss.Style
	ch     chan PrefixMessage
	w      io.Writer
}

func (p *prefixWrapper) Write(payload []byte) (int, error) {
	p.ch <- PrefixMessage{
		w:        p.w,
		style:    p.style,
		b:        bytes.Clone(payload),
		producer: p.prefix,
	}
	return len(payload), nil
}
