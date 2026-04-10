package run

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"text/tabwriter"

	"charm.land/lipgloss/v2"
	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/setup/flagset"
	"github.com/raffis/rageta/internal/styles"
)

type SummaryOptions struct {
	SkipSummary bool
}

func (s *SummaryOptions) BindFlags(flags flagset.Interface) {
	flags.BoolVarP(&s.SkipSummary, "skip-summary", "", s.SkipSummary, "Do not print an execution summary at the end of the pipeline execution.")
}

func (s SummaryOptions) Build() Step {
	return &Summary{opts: s}
}

type Summary struct {
	opts SummaryOptions
}

func (s *Summary) Run(rc *RunContext, next Next) error {
	err := next(rc)

	if s.opts.SkipSummary {
		return err
	}

	if err != nil {
		s.writeErrorToStderr(err, rc)
		return nil
	}

	s.writeSuccessToStderr(rc)
	return nil
}

func (s *Summary) writeErrorToStderr(err error, rc *RunContext) {
	var pipelineExecErr *pipelineExecutionError
	if errors.As(err, &pipelineExecErr) {
		s.writePipelineErrorToStderr(errors.Unwrap(err), []error{errors.Unwrap(err)}, rc)
	} else {
		fmt.Fprintln(rc.Output.Stderr, styles.Highlight.Render("Details:"))
		fmt.Fprintln(rc.Output.Stderr, err.Error())
	}

	helpCmd := "rageta help"

	if rc.Provider.Ref != "" {
		helpCmd = fmt.Sprintf("%s %s", helpCmd, rc.Provider.Ref)
	}
	fmt.Fprintf(rc.Output.Stderr, "\nRun %s for more information\n", styles.HelpSection.Render(helpCmd))
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
	var stepErr processor.StepError
	if errors.As(err, &stepErr) {
		fmt.Fprintf(rc.Output.Stderr, "The step %s failed.\n\n", styles.HelpSection.Render(stepErr.StepName()))
	}

	var tags []string
	w := tabwriter.NewWriter(rc.Output.Stderr, 0, 0, 2, ' ', 0)
	var innerStepErr processor.StepError
	if AsInner(err, &innerStepErr) {
		fmt.Fprintf(w, "%s\t%s\n", styles.Highlight.Render("Inner Step:"), innerStepErr.StepName())
		fmt.Fprintf(w, "%s\t%s\n", styles.Highlight.Render("Context path:"), path.Join(rc.ContextDir.Path, innerStepErr.Context().UniqueID()))

		for _, tag := range innerStepErr.Context().Tags.Tags() {
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
	} else {
		fmt.Fprintf(w, "%s\t%s\n", styles.Highlight.Render("Error:"), errors.Unwrap(err).Error())
	}

	fmt.Fprint(w, "\n")
	w.Flush()

	fmt.Fprintf(w, "%s\n", styles.Highlight.Render("Trace:"))
	i := 0
	for _, parentErr := range parents {
		var stepErr processor.StepError
		if errors.As(parentErr, &stepErr) {
			fmt.Fprintln(rc.Output.Stderr, styles.Highlight.Render(fmt.Sprintf("#%d step %s failed", i, stepErr.StepName())))
		}

		i++
	}
}

func (s *Summary) writeSuccessToStderr(rc *RunContext) {
	fmt.Fprintf(rc.Output.Stderr, "\nThe pipeline was successfully executed.\n\n")
	w := tabwriter.NewWriter(rc.Output.Stderr, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "%s\t%s\n", styles.Highlight.Render("Context path:"), rc.ContextDir.Path)
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
