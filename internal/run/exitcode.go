package run

import (
	"fmt"
	"os"

	"github.com/raffis/rageta/internal/setup/flagset"
)

type ExitCodeOptions struct {
	AllowFailure bool
}

func (s *ExitCodeOptions) BindFlags(flags flagset.Interface) {
	flags.BoolVarP(&s.AllowFailure, "allow-failure", "", s.AllowFailure, "In case of an error exit with code 0.")
}

func (s ExitCodeOptions) Build() Task {
	return &ExitCode{opts: s}
}

type ExitCode struct {
	opts ExitCodeOptions
}

func (s *ExitCode) Run(rc *RunContext, next Next) error {
	err := next(rc)
	if err != nil {
		fmt.Fprintf(rc.Display.Stderr, "\n%s %s", "ERROR", err.Error())

    if s.opts.AllowFailure {
      os.Exit(0)
    }

		os.Exit(1)
	}

	return nil
}
