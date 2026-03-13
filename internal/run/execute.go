package run

import (
	"context"

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

func (s *Execute) Run(rc *RunContext, next Next) error {
	stepCtx := processor.NewContext()
	stepCtx.Context = rc.Context

	pipelineCmd, err := rc.Pipeline.Builder.Build(rc.Provider.Pipeline, s.opts.Entrypoint, rc.Inputs.Args, stepCtx)
	if err != nil {
		//rc.Result = err
		//return next(rc)
		return err
	}

	err = s.retryRun(rc.Context, pipelineCmd)
	if err != nil {
		//rc.Result = err
		//return next(rc)
		return err
	}

	return next(rc)
}

func (s *Execute) retryRun(ctx context.Context, pipelineCmd processor.Executable) error {
	var backoff retry.Backoff
	return retry.Do(ctx, retry.WithMaxRetries(s.opts.MaxRetries, backoff), func(ctx context.Context) error {
		_, _, err := pipelineCmd()
		if err != nil {
			return retry.RetryableError(err)
		}
		return nil
	})
}
