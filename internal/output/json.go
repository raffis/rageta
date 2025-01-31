package output

import (
	"io"

	"github.com/go-logr/zapr"
	"github.com/raffis/rageta/internal/processor"
	"go.uber.org/zap"
)

func JSON() processor.OutputFactory {
	return func(name string, stdin io.Reader, stdout, stderr io.Writer) (io.Reader, io.Writer, io.Writer, processor.OutputCloser) {
		stdout, stderr = jsonWriter(name, stdout, stderr)

		return stdin, stdout, stderr, func(err error) {}
	}
}

func jsonWriter(task string, stdout, stderr io.Writer) (io.Writer, io.Writer) {
	zapLog := zap.Must(zap.NewProduction())

	if stdout != nil {
		stdout = NewLogWriter(zapr.NewLogger(zapLog).WithValues("task", task, "dev", "/dev/stdout"))
	}
	if stderr != nil {
		stderr = NewLogWriter(zapr.NewLogger(zapLog).WithValues("task", task, "dev", "/dev/stderr"))
	}

	return stdout, stderr
}
