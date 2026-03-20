package run

import (
	"errors"
	"fmt"
	"os"

	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/styles"
)

type ErrorOptions struct {
}

func (s ErrorOptions) Build() Step {
	return &Error{opts: s}
}

type Error struct {
	opts ErrorOptions
}

func (s *Error) Run(rc *RunContext, next Next) error {
	err := next(rc)
	if err != nil {
		s.writeErrorToStderr(err, rc)
	}

	return nil
}

func (s *Error) writeErrorToStderr(err error, rc *RunContext) {
	fmt.Fprintln(os.Stderr, "")
	var stepErr processor.ErrorGetStepName
	if errors.As(err, &stepErr) {
		fmt.Fprintf(os.Stderr, "The step %s failed.\n", styles.Highlight.Render(stepErr.StepName()))
	}

	var runErr processor.ErrorContainer
	if errors.As(err, &runErr) {
		fmt.Fprintf(os.Stderr, "Container: %s\n", styles.Highlight.Render(runErr.ContainerName()))
		fmt.Fprintf(os.Stderr, "Image: %s\n", styles.Highlight.Render(runErr.Image()))
		fmt.Fprintf(os.Stderr, "Exit Code: %s\n", styles.Highlight.Render(fmt.Sprintf("%d", runErr.ExitCode())))
	}

	fmt.Fprintln(os.Stderr, styles.HelpSection.Render("\nDetails:"))
	fmt.Fprintln(os.Stderr, err.Error())
	helpCmd := "rageta help"

	if rc.Provider.Ref != "" {
		helpCmd = fmt.Sprintf("%s %s", helpCmd, rc.Provider.Ref)
	}
	fmt.Fprintf(os.Stderr, "\nRun %s for more information\n", styles.Highlight.Render(helpCmd))
}
