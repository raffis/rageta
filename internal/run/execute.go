package run

import (
	"context"
	"fmt"
	"time"

	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/setup/flagset"
	"github.com/sethvargo/go-retry"
)

type ExecuteOptions struct {
	MaxRetries uint64
	Entrypoint string
}

func (s *ExecuteOptions) BindFlags(flags flagset.Interface) {
	flags.Uint64VarP(&s.MaxRetries, "retry", "", s.MaxRetries, "Retry pipeline if a failure occurred.")
	flags.StringVarP(&s.Entrypoint, "entrypoint", "t", s.Entrypoint, "Entrypoint for the given pipeline. The pipelines default is used otherwise.")
}

func (s ExecuteOptions) Build() Task {
	return &Execute{opts: s}
}

type Execute struct {
	opts ExecuteOptions
}

type pipelineExecutionError struct {
	parent error
}

type ExecuteContext struct {
	ResultContext processor.TaskContext
}

func (e *pipelineExecutionError) Error() string {
	return fmt.Sprintf("pipeline execution failed: %s", e.parent.Error())
}

func (e *pipelineExecutionError) Unwrap() error {
	return e.parent
}

func (s *Execute) Label() string {
	return "Executing pipeline"
}

func (s *Execute) Run(rc *RunContext, next Next) error {
	stepContext := processor.NewContext()
	stepContext.Context = rc.Context

	pipelineCmd, err := rc.Pipeline.Builder.Build(rc.Provider.Pipeline, s.opts.Entrypoint, rc.Inputs.Args, stepContext)
	if err != nil {
		return err
	}

	err = s.retryRun(rc, pipelineCmd)
	if err != nil {
		return &pipelineExecutionError{err}
	}

	return next(rc)
}

func (s *Execute) retryRun(rc *RunContext, pipelineCmd processor.Executable) error {
	var inner retry.Backoff = retry.BackoffFunc(func() (time.Duration, bool) { return 0, true })
	if s.opts.MaxRetries > 0 {
		inner = retry.NewConstant(time.Second)
	}
	b := retry.WithMaxRetries(s.opts.MaxRetries, inner)

	if err := rc.Checklist.Close(); err != nil {
		return fmt.Errorf("closing checklist failed: %w", err)
	}

	return retry.Do(rc.Context, b, func(ctx context.Context) error {
		var err error
		rc.Execute.ResultContext, _, err = pipelineCmd()

		if err != nil {
			return retry.RetryableError(err)
		}

		return nil
	})
}
