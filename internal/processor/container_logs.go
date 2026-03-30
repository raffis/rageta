package processor

import (
	"fmt"
	"io"
	"os"
	"path"
	"slices"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithContainerLogs(enabled bool) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if !enabled {
			return nil
		}

		logs := &ContainerLogs{
			stepName: spec.Name,
		}
		return logs
	}
}

type ContainerLogs struct {
	stepName string
}

func (s *ContainerLogs) Bootstrap(pipelineCtx Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		stdoutPath := path.Join(ctx.ContextDir, ctx.UniqueID, "0.out")

		stdout, err := os.OpenFile(stdoutPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
		if err != nil {
			return ctx, fmt.Errorf("failed to add container stdout logs: %w", err)
		}

		defer func() {
			_ = stdout.Close()
		}()

		ctx.Streams.AdditionalStdout = append(ctx.Streams.AdditionalStdout, stdout)

		stderrPath := path.Join(ctx.ContextDir, ctx.UniqueID, "1.out")

		stderr, err := os.OpenFile(stderrPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
		if err != nil {
			return ctx, fmt.Errorf("failed to add container stderr logs: %w", err)
		}

		defer func() {
			_ = stderr.Close()
		}()

		ctx.Streams.AdditionalStderr = append(ctx.Streams.AdditionalStderr, stderr)

		ctx, err = next(ctx)
		ctx.Streams.AdditionalStdout = slices.DeleteFunc(ctx.Streams.AdditionalStdout, func(w io.Writer) bool {
			return w == stdout
		})
		ctx.Streams.AdditionalStderr = slices.DeleteFunc(ctx.Streams.AdditionalStderr, func(w io.Writer) bool {
			return w == stderr
		})

		return ctx, err
	}, nil
}
