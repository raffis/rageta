package run

import (
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"

	"charm.land/lipgloss/v2"
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
	var stepErr processor.StepError
	if errors.As(err, &stepErr) {
		fmt.Fprintf(rc.Output.Stderr, "\nThe step %s failed.\n\n", styles.HelpSection.Render(stepErr.StepName()))
	}

	var tags []string
	w := tabwriter.NewWriter(rc.Output.Stderr, 0, 0, 2, ' ', 0)
	var innerStepErr processor.StepError
	if AsInner(err, &innerStepErr) {
		fmt.Fprintf(w, "%s\t%s\n", styles.Highlight.Render("Inner Step:"), innerStepErr.StepName())
		fmt.Fprintf(w, "%s\t%s\n", styles.Highlight.Render("Context path:"), rc.ContextDir.Path)

		for _, tag := range innerStepErr.Context().Tags() {
			tags = append(tags, styles.TagLabel.
				Background(lipgloss.Color(tag.Color)).
				Foreground(styles.AdaptiveBrightnessColor(lipgloss.Color(tag.Color))).
				Render(fmt.Sprintf("%s: %s", tag.Key, tag.Value)),
			)
		}
	}

	var runErr processor.ErrorContainer
	if errors.As(err, &runErr) {
		fmt.Fprintf(w, "%s\t%s\n", styles.Highlight.Render("Container:"), runErr.ContainerName())
		fmt.Fprintf(w, "%s\t%s\n", styles.Highlight.Render("Image:"), runErr.Image())
		fmt.Fprintf(w, "%s\t%d\n", styles.Highlight.Render("Exit Code:"), runErr.ExitCode())

		if len(tags) > 0 {
			fmt.Fprintf(w, "%s\t%s\n", styles.Highlight.Render("Tags:"), strings.Join(tags, " "))

		}

		fmt.Fprint(w, "\n")
	}

	w.Flush()

	fmt.Fprintln(rc.Output.Stderr, styles.Highlight.Render("Details:"))
	fmt.Fprintln(rc.Output.Stderr, err.Error())
	helpCmd := "rageta help"

	if rc.Provider.Ref != "" {
		helpCmd = fmt.Sprintf("%s %s", helpCmd, rc.Provider.Ref)
	}
	fmt.Fprintf(rc.Output.Stderr, "\nRun %s for more information\n", styles.HelpSection.Render(helpCmd))
}

func AsInner(err error, target any) bool {
	var found bool
	for err != nil {
		if errors.As(err, target) {
			found = true
		}
		err = errors.Unwrap(err)
	}
	return found
}
