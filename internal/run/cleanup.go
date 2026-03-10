package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/raffis/rageta/internal/pipeline"
	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/styles"
	"github.com/raffis/rageta/internal/tui"
)

type Cleanup struct {
	contextDir          string
	noGC                bool
	gracefulTermination time.Duration
}

func WithCleanup(contextDir string, noGC bool, gracefulTermination time.Duration) *CleanupStep {
	return &CleanupStep{contextDir: contextDir, noGC: noGC, gracefulTermination: gracefulTermination}
}

func (s *Cleanup) Run(rc *RunContext, next Next) error {
	if rc.PersistDB != nil {
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

	if s.contextDir == "" && !s.noGC {
		_ = os.RemoveAll(rc.ContextDir)
	}

	if rc.ReportFactory != nil {
		if err := rc.ReportFactory.Finalize(); err != nil {
			rc.Result = errors.Join(rc.Result, err)
		}
	}

	if rc.monitorFile != nil {
		_ = rc.monitorFile.Close()
	}

	return next(rc)
}

func (s *CleanupStep) runTeardown(rc *RunContext) {
	rc.Cancel()
	if rc.Teardown != nil {
		ch := rc.Teardown
		rc.Teardown = nil
		close(ch)
	}

	teardownCtx, cancel := context.WithTimeout(context.Background(), s.gracefulTermination+time.Second)
	defer cancel()

	var wg sync.WaitGroup
	for _, fn := range rc.TeardownFuncs {
		wg.Add(1)
		go func(fn processor.Teardown) {
			defer wg.Done()
			rc.Logger.V(5).Info("execute teardown")
			if err := fn(teardownCtx, s.gracefulTermination); err != nil {
				rc.Logger.V(5).Info("failed execute teardown", "err", err)
			}
		}(fn)
	}
	wg.Wait()
}

// WriteErrorToStderr writes the run result error to stderr.
func WriteErrorToStderr(rc *RunContext) {
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
