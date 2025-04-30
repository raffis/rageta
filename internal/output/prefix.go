package output

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/raffis/rageta/internal/processor"
)

func TerminalWriter(ch chan PrefixMessage) {
	for msg := range ch {
		lines := strings.Split(strings.ReplaceAll(strings.TrimSuffix(string(msg.b), "\n"), "\r", ""), "\n")
		for _, line := range lines {
			msg.w.Write([]byte(msg.style.Render(msg.producer)))
			msg.w.Write([]byte(line))
			msg.w.Write([]byte{'\n'})
		}
	}
}

func Prefix(color bool, stdout, stderr io.Writer, ch chan PrefixMessage) processor.OutputFactory {
	return func(_ context.Context, stepContext processor.StepContext, stepName string) (io.Writer, io.Writer, processor.OutputCloser) {
		style := lipgloss.NewStyle()

		if color {
			style = style.Foreground(lipgloss.AdaptiveColor{
				Dark:  randHEXColor(127, 255),
				Light: randHEXColor(0, 127),
			})
		}

		stdoutWrapper := &prefixWrapper{
			prefix: fmt.Sprintf("%s ", stepName),
			style:  style,
			ch:     ch,
			w:      stdout,
		}

		stderrWrapper := &prefixWrapper{
			prefix: fmt.Sprintf("%s ", stepName),
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

func randHEXColor(min, max int) string {
	R := rand.Intn(max-min+1) + min
	G := rand.Intn(max-min+1) + min
	B := rand.Intn(max-min+1) + min
	return fmt.Sprintf("#%02x%02x%02x", R, G, B)
}
