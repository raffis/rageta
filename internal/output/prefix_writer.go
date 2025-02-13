package output

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"sync"

	"github.com/charmbracelet/lipgloss"
)

type Prefixer struct {
	prefix          string
	style           lipgloss.Style
	writer          io.Writer
	trailingNewline bool
	buf             bytes.Buffer
	lock            *lockInfo
}

type PrefixOptions struct {
	Prefix string
	Style  lipgloss.Style
	Lock   *lockInfo
}

func NewPrefixWriter(writer io.Writer, opts PrefixOptions) *Prefixer {
	return &Prefixer{
		writer:          writer,
		trailingNewline: true,
		prefix:          opts.Prefix,
		style:           opts.Style,
		lock:            opts.Lock,
	}
}

type lockInfo struct {
	mu *sync.Mutex
	id string
}

func (p *Prefixer) Close() error {
	if p.lock.id == p.prefix {
		p.lock.id = ""
		p.lock.mu.Unlock()
	}

	return nil
}

func (p *Prefixer) Write(payload []byte) (int, error) {
	if p.lock.id == "" {
		p.lock.mu.Lock()
		p.lock.id = p.prefix
	}

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

	bytes := p.buf.Bytes()
	if bytes[len(bytes)-1] == '\n' {
		defer func() {
			p.lock.id = ""
			p.lock.mu.Unlock()
		}()
	}

	n, err := p.writer.Write(bytes)
	if err != nil {
		if n > len(payload) {
			n = len(payload)
		}
		return n, err
	}

	return len(payload), nil
}

func prefixWriter(prefix string, stdout, stderr io.Writer, randColor bool, lockInfo *lockInfo) (io.Writer, io.Writer) {
	style := lipgloss.NewStyle()

	if randColor {
		color, _ := randomHex(6)
		style = style.Foreground(lipgloss.Color(fmt.Sprintf("#%s", color)))
	}

	if stdout != nil {
		stdout = NewPrefixWriter(stdout, PrefixOptions{
			Prefix: fmt.Sprintf("%s ", prefix),
			Style:  style,
			Lock:   lockInfo,
		})
	}

	if stderr != nil {
		stderr = NewPrefixWriter(stderr, PrefixOptions{
			Prefix: fmt.Sprintf("%s ", prefix),
			Style:  style,
			Lock:   lockInfo,
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
