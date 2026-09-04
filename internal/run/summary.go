package run

import (
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"

	"charm.land/lipgloss/v2"
	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/setup/flagset"
	"github.com/raffis/rageta/internal/styles"
)

type SummaryOptions struct {
	NoSummary bool
}

func (s *SummaryOptions) BindFlags(flags flagset.Interface) {
	flags.BoolVarP(&s.NoSummary, "no-summary", "", s.NoSummary, "Do not print an execution summary at the end of the pipeline execution.")
}

func (s SummaryOptions) Build() Task {
	return &Summary{opts: s}
}

type Summary struct {
	opts SummaryOptions
}

func (s *Summary) Label() string {
	return "Preparing summary"
}

func (s *Summary) Run(rc *RunContext, next Next) error {
	err := next(rc)

	if s.opts.NoSummary {
		return err
	}

	if err != nil {
		s.writeErrorToStderr(err, rc)
		return err
	}

	s.writeSuccessToStderr(rc)
	return err
}

func (s *Summary) writeErrorToStderr(err error, rc *RunContext) {
	var pipelineExecErr *pipelineExecutionError
	if errors.As(err, &pipelineExecErr) {
		s.writePipelineErrorToStderr(errors.Unwrap(err), []error{errors.Unwrap(err)}, rc)
	} else {
		fmt.Fprintln(rc.Display.Stderr, styles.Highlight.Render("Details:"))
		fmt.Fprintln(rc.Display.Stderr, err.Error())
	}

	helpCmd := "rageta help"

	if rc.Provider.Ref != "" {
		helpCmd = fmt.Sprintf("%s %s", helpCmd, rc.Provider.Ref)
	}
	fmt.Fprintf(rc.Display.Stderr, "\nRun %s for more information\n", styles.HelpSection.Render(helpCmd))
}

func (s *Summary) writePipelineErrorToStderr(err error, parents []error, rc *RunContext) {
	unwrappedErr := err
	for unwrappedErr != nil {
		if uw, ok := unwrappedErr.(interface{ Unwrap() []error }); ok {
			for _, unwrappedErr := range uw.Unwrap() {
				s.writePipelineErrorToStderr(unwrappedErr, append(parents, unwrappedErr), rc)
			}

			return
		}

		unwrappedErr = errors.Unwrap(unwrappedErr)
	}

	fmt.Printf("\n───────\n")
	var stepErr processor.TaskError
	if errors.As(err, &stepErr) {
		fmt.Fprintf(rc.Display.Stderr, "The step %s failed.\n\n", styles.HelpSection.Render(stepErr.TaskName()))
	}

	var labels []string
	w := tabwriter.NewWriter(rc.Display.Stderr, 0, 0, 2, ' ', 0)
	var innerTaskErr processor.TaskError
	if AsInner(err, &innerTaskErr) {
		fmt.Fprintf(w, "%s\t%s\n", styles.Highlight.Render("Inner Task:"), innerTaskErr.TaskName())

		for _, label := range innerTaskErr.Context().Labels.Labels() {
			labels = append(labels, styles.Label.
				Background(lipgloss.Color(label.HEXColor)).
				Foreground(styles.AdaptiveBrightnessColor(lipgloss.Color(label.HEXColor))).
				Render(fmt.Sprintf("%s: %s", label.Key, label.Value)),
			)
		}
	}

	var imageErr processor.ImageName
	if errors.As(err, &imageErr) {
		fmt.Fprintf(w, "%s\t%s\n", styles.Highlight.Render("Image:"), imageErr.Image())
	}

	var exitCodeErr processor.ExitCode
	switch {
	case errors.As(err, &exitCodeErr):
		fmt.Fprintf(w, "%s\t%d\n", styles.Highlight.Render("Exit Code:"), exitCodeErr.ExitCode())
	case errors.Unwrap(err) != nil:
		fmt.Fprintf(w, "%s\t%s\n", styles.Highlight.Render("Error:"), errors.Unwrap(err).Error())
	default:
		fmt.Fprintf(w, "%s\t%s\n", styles.Highlight.Render("Error:"), err.Error())
	}

	if len(labels) > 0 {
		fmt.Fprintf(w, "%s\t%s\n", styles.Highlight.Render("Labels:"), strings.Join(labels, " "))
	}

	fmt.Fprint(w, "\n")
	w.Flush()

	fmt.Fprintf(w, "%s\n", styles.Highlight.Render("Trace:"))
	i := 0
	for _, parentErr := range parents {
		var stepErr processor.TaskError
		if errors.As(parentErr, &stepErr) {
			fmt.Fprintln(rc.Display.Stderr, styles.Highlight.Render(fmt.Sprintf("#%d step %s failed", i, stepErr.TaskName())))
		}

		i++
	}
}

func (s *Summary) writeSuccessToStderr(rc *RunContext) {
	fmt.Fprintf(rc.Display.Stderr, "\nThe pipeline was successfully executed.\n\n")
	w := tabwriter.NewWriter(rc.Display.Stderr, 0, 0, 2, ' ', 0)
	//fmt.Fprintf(w, "%s\t%s\n", styles.Highlight.Render("Context path:"), rc.ContextDir.Path)
	w.Flush()
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
