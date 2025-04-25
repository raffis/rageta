package output

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/charmbracelet/lipgloss"
)

type Prefixer struct {
	prefix          string
	style           lipgloss.Style
	writer          chan prefixMessage
	trailingNewline bool
	buf             bytes.Buffer
}

type PrefixOptions struct {
	Prefix string
	Style  lipgloss.Style
}

func NewPrefixWriter(writer chan prefixMessage, opts PrefixOptions) *Prefixer {
	return &Prefixer{
		writer:          writer,
		trailingNewline: true,
		prefix:          opts.Prefix,
		style:           opts.Style,
	}
}

func (p *Prefixer) Write(payload []byte) (int, error) {
	p.buf.Reset()

	for _, b := range payload {
		if p.trailingNewline {
			p.buf.WriteString(p.style.Render(p.prefix))
			p.trailingNewline = false
		}

		p.buf.WriteByte(b)
		if b == '\n' {
			p.trailingNewline = true
		}
	}

	p.writer <- prefixMessage{
		b:        bytes.Clone(p.buf.Bytes()),
		producer: p.prefix,
	}
	return len(payload), nil
}

func prefixWriter(prefix string, stdoutCh, stderrCh chan prefixMessage, randColor bool) (io.Writer, io.Writer) {
	style := lipgloss.NewStyle()

	if randColor {
		color, _ := randomHex(6)
		style = style.Foreground(lipgloss.Color(fmt.Sprintf("#%s", color)))
	}

	var stdout, stderr io.Writer

	if stdoutCh != nil {
		stdout = NewPrefixWriter(stdoutCh, PrefixOptions{
			Prefix: fmt.Sprintf("%s ", prefix),
			Style:  style,
		})
	}

	if stderrCh != nil {
		stderr = NewPrefixWriter(stderrCh, PrefixOptions{
			Prefix: fmt.Sprintf("%s ", prefix),
			Style:  style,
		})
	}

	return stdout, stderr
}

func randomHex(n int) (string, error) {
	bytes := make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
