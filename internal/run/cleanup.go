package run

import (
	"time"
)

type CleanupOptions struct {
	ContextDir          string
	NoGC                bool
	GracefulTermination time.Duration
}

func (s CleanupOptions) Build() Step {
	return &Cleanup{opts: s}
}

type Cleanup struct {
	opts CleanupOptions
}

func (s *Cleanup) Run(rc *RunContext, next Next) error {
	/*if rc.PersistDB != nil {
		if err := rc.PersistDB(); err != nil {
			rc.Logger.V(1).Error(err, "failed to persist database")
		}
	}

	if rc.TUIApp != nil {
		app, _ := rc.TUIApp.(interface {
			Quit()
			Send(interface{})
		})
		if app != nil {
			if errors.Is(rc.Result, pipeline.ErrInvalidInput) {
				app.Quit()
			}
			if rc.Result != nil {
				app.Send(tui.PipelineDoneMsg{Status: tui.StepStatusFailed, Error: rc.Result})
			} else {
				app.Send(tui.PipelineDoneMsg{Status: tui.StepStatusDone, Error: nil})
			}
		}
	}
	s.runTeardown(rc)
	if rc.TUIApp != nil {
		<-rc.TUIDone
	}

	if s.opts.ContextDir == "" && !s.opts.NoGC {
		_ = os.RemoveAll(rc.ContextDir)
	}

	if rc.monitorFile != nil {
		_ = rc.monitorFile.Close()
	}
	*/
	return next(rc)
}

// WriteErrorToStderr writes the run result error to stderr.
/*func WriteErrorToStderr(rc *RunContext) {
	if rc.Result == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "")
	var stepErr processor.ErrorGetStepName
	if errors.As(rc.Result, &stepErr) {
		fmt.Fprintf(os.Stderr, "The step %s failed.\n", styles.Highlight.Render(stepErr.StepName()))
	}
	fmt.Fprintln(os.Stderr, styles.HelpSection.Render("\nDetails:"))
	fmt.Fprintln(os.Stderr, rc.Result.Error())
	helpCmd := "rageta help"
	if rc.Input.Ref != "" {
		helpCmd = fmt.Sprintf("%s %s", helpCmd, rc.Input.Ref)
	}
	fmt.Fprintf(os.Stderr, "\nRun %s for more information\n", styles.Highlight.Render(helpCmd))
}
*/
