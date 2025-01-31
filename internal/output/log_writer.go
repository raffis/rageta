package output

import (
	"bytes"

	"github.com/go-logr/logr"
)

type Logger struct {
	logger          logr.Logger
	trailingNewline bool
	buf             bytes.Buffer
}

func NewLogWriter(logger logr.Logger) *Logger {
	return &Logger{
		logger: logger,
	}
}

func (l *Logger) Write(payload []byte) (int, error) {
	l.buf.Reset()

	for _, b := range payload {

		l.buf.WriteByte(b)

		if b == '\n' {
			l.trailingNewline = true
		}
	}

	if l.trailingNewline {
		b := l.buf.Bytes()
		l.logger.Info(string(b))
		l.trailingNewline = false
		return len(b), nil
	}

	return len(payload), nil
}
