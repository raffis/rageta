package run

import (
	"os"

	"github.com/raffis/rageta/internal/setup/flagset"
)

type ExitCodeOptions struct {
	ExitCode string
}

func (s *ExitCodeOptions) BindFlags(flags flagset.Interface) {
	flags.StringVarP(&s.ExitCode, "x", "", s.ExitCode, ".")
}

func (s ExitCodeOptions) Build() Step {
	return &ExitCode{opts: s}
}

type ExitCode struct {
	opts ExitCodeOptions
}

func (s *ExitCode) Run(rc *RunContext, next Next) error {
	err := next(rc)
	if err != nil {
		os.Exit(1)
	}

	return nil
}
