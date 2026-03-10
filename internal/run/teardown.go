package runner

import (
	"io"
	"os"

	"github.com/alitto/pond/v2"
	"github.com/raffis/rageta/internal/processor"
)

type TeardownStep struct {
	maxConcurrent int
	statusOutput  string
	output        string
}

func WithTeardown(maxConcurrent int, statusOutput, output string) *TeardownStep {
	return &TeardownStep{maxConcurrent: maxConcurrent, statusOutput: statusOutput, output: output}
}

func (s *TeardownStep) Run(rc *RunContext, next Next) error {
	teardown := make(chan processor.Teardown)
	rc.Teardown = teardown

	go func() {
		for fn := range teardown {
			rc.TeardownFuncs = append(rc.TeardownFuncs, fn)
		}
	}()

	rc.Pool = pond.NewPool(s.maxConcurrent)
	rc.Logger.V(3).Info("worker pool", "max-concurrency", s.maxConcurrent)

	rc.MonitorDev = s.monitorDevice(rc)
	return next(rc)
}

func (s *TeardownStep) monitorDevice(rc *RunContext) io.Writer {
	switch {
	case s.statusOutput == "/dev/stdout" || s.statusOutput == "-":
		return rc.Stdout
	case s.statusOutput == "/dev/stderr":
		return rc.Stderr
	case s.statusOutput != "":
		f, err := os.OpenFile(s.statusOutput, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0640)
		if err != nil {
			return nil
		}
		rc.monitorFile = f
		return f
	case s.output == renderOutputDiscard || s.output == renderOutputPassthrough:
		return rc.Stdout
	default:
		return nil
	}
}
