package runner

import (
	"context"

	"github.com/raffis/rageta/internal/processor"
	"github.com/sethvargo/go-retry"
)

type ExecuteStep struct {
	maxRetries uint64
}

func WithExecute(maxRetries uint64) *ExecuteStep {
	return &ExecuteStep{maxRetries: maxRetries}
}

func (s *ExecuteStep) Run(rc *RunContext, next Next) error {
	stepCtx := processor.NewContext()
	stepCtx.Context = rc.Ctx

	pipelineCmd, err := rc.Builder.Build(rc.Command, rc.Input.Entrypoint, rc.Inputs, stepCtx)
	if err != nil {
		rc.Result = err
		return next(rc)
	}

	rc.Result = s.retryRun(rc.Ctx, pipelineCmd)
	return next(rc)
}

func (s *ExecuteStep) retryRun(ctx context.Context, pipelineCmd processor.Executable) error {
	var backoff retry.Backoff
	return retry.Do(ctx, retry.WithMaxRetries(s.maxRetries, backoff), func(ctx context.Context) error {
		_, _, err := pipelineCmd()
		if err != nil {
			return retry.RetryableError(err)
		}
		return nil
	})
}
