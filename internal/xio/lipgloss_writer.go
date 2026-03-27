package xio

import (
	"fmt"
	"io"

	"charm.land/lipgloss/v2"
)

func NewLipglossWriter(w io.Writer, style lipgloss.Style) *LipglossWriter {
	return &LipglossWriter{
		w:     w,
		style: style,
	}
}

type LipglossWriter struct {
	w     io.Writer
	style lipgloss.Style
}

func (sw LipglossWriter) Write(p []byte) (int, error) {
	line := string(p)
	var suffix string

	if p[len(p)-1] == '\n' {
		line = line[:len(line)-1]
		suffix = "\n"
	}

	return fmt.Fprint(sw.w, sw.style.Render(line)+suffix)
}
