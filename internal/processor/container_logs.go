package processor

import (
	"fmt"
	"io"
	"os"
	"path"
	"slices"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithContainerLogs(enabled bool, wrap secretMaskWrapper) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if !enabled || spec.Run == nil {
			return nil
		}

		logs := &ContainerLogs{
			stepName: spec.Name,
			wrap:     wrap,
		}
		return logs
	}
}

type secretMaskWrapper interface {
	Writer(io.Writer) io.Writer
}

type ContainerLogs struct {
	stepName string
	wrap     secretMaskWrapper
}

func (s *ContainerLogs) Bootstrap(pipelineCtx Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		stdoutPath := path.Join(ctx.ContextDir, ctx.UniqueID(), "stdout.out")

		stdout, err := os.OpenFile(stdoutPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
		if err != nil {
			return ctx, fmt.Errorf("failed to add container stdout logs: %w", err)
		}

		defer func() {
			_ = stdout.Close()
		}()

		maskedStdout := s.wrap.Writer(stdout)
		ctx.Streams.AdditionalStdout = append(ctx.Streams.AdditionalStdout, maskedStdout)

		stderrPath := path.Join(ctx.ContextDir, ctx.UniqueID(), "stderr.out")

		stderr, err := os.OpenFile(stderrPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
		if err != nil {
			return ctx, fmt.Errorf("failed to add container stderr logs: %w", err)
		}

		defer func() {
			_ = stderr.Close()
		}()

		maskedStderr := s.wrap.Writer(stderr)
		ctx.Streams.AdditionalStderr = append(ctx.Streams.AdditionalStderr, maskedStderr)

		ctx, err = next(ctx)
		ctx.Streams.AdditionalStdout = slices.DeleteFunc(ctx.Streams.AdditionalStdout, func(w io.Writer) bool {
			return w == maskedStdout
		})
		ctx.Streams.AdditionalStderr = slices.DeleteFunc(ctx.Streams.AdditionalStderr, func(w io.Writer) bool {
			return w == maskedStderr
		})

		return ctx, err
	}, nil
}
