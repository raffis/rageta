package run

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/raffis/rageta/internal/processor"
	"github.com/sethvargo/go-retry"
	"github.com/spf13/pflag"
)

type ExecuteOptions struct {
	MaxRetries uint64
	Entrypoint string
}

func (s *ExecuteOptions) BindFlags(flags *pflag.FlagSet) {
	flags.Uint64VarP(&s.MaxRetries, "retry", "", 0, "Retry pipeline if a failure occurred.")
	flags.StringVarP(&s.Entrypoint, "entrypoint", "t", s.Entrypoint, "Entrypoint for the given pipeline. The pipelines default is used otherwise.")
}

func (s ExecuteOptions) Build() Step {
	return &Execute{opts: s}
}

type Execute struct {
	opts ExecuteOptions
}

type ExecutionContext struct {
	StepContext processor.StepContext
}

var PipelineSetupError = errors.New("pipeline setup failed")

func (s *Execute) Run(rc *RunContext, next Next) error {
	rc.Execution.StepContext.Context = rc.Context

	pipelineCmd, err := rc.Pipeline.Builder.Build(rc.Provider.Pipeline, s.opts.Entrypoint, rc.Inputs.Args, rc.Execution.StepContext)
	if err != nil {
		return fmt.Errorf("%w: %w", PipelineSetupError, err)
	}

	err = s.retryRun(rc.Context, pipelineCmd)
	if err != nil {
		return err
	}

	return next(rc)
}

func (s *Execute) retryRun(ctx context.Context, pipelineCmd processor.Executable) error {
	var inner retry.Backoff = retry.BackoffFunc(func() (time.Duration, bool) { return 0, true })
	if s.opts.MaxRetries > 0 {
		inner = retry.NewConstant(time.Second)
	}
	b := retry.WithMaxRetries(s.opts.MaxRetries, inner)
	return retry.Do(ctx, b, func(ctx context.Context) error {
		_, _, err := pipelineCmd()
		if err != nil {
			return retry.RetryableError(err)
		}
		return nil
	})
}
