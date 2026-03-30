package processor

import (
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithStdioRedirect(tee bool) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Streams == nil {
			return nil
		}

		stdio := &StdioRedirect{
			streams: spec.Streams,
			tee:     tee,
		}

		return stdio
	}
}

type StdioRedirect struct {
	streams *v1beta1.Streams
	tee     bool
}

func (s *StdioRedirect) Bootstrap(pipelineCtx Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		var stdoutRedirect, stderrRedirect io.Writer
		streams := s.streams.DeepCopy()

		vars := []any{}
		if streams.Stdout != nil {
			vars = append(vars, &streams.Stdout.Path)
		}

		if streams.Stderr != nil {
			vars = append(vars, &streams.Stderr.Path)
		}
		if streams.Stdin != nil {
			vars = append(vars, &streams.Stdin.Path)
		}

		if err := substitute.Substitute(ctx.ToV1Beta1(), vars...,
		); err != nil {
			return ctx, err
		}

		stdout := ctx.Streams.Stdout
		stderr := ctx.Streams.Stderr

		if streams.Stdout != nil {
			if !s.tee {
				ctx.Streams.Stdout = io.Discard
			}

			outFile, err := os.OpenFile(streams.Stdout.Path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
			if err != nil {
				return ctx, fmt.Errorf("failed to redirect stdout: %w", err)
			}

			defer func() {
				_ = outFile.Close()
			}()
			ctx.Streams.AdditionalStdout = append(ctx.Streams.AdditionalStdout, outFile)
			stdoutRedirect = outFile
		}

		if streams.Stderr != nil {
			if !s.tee {
				ctx.Streams.Stdout = io.Discard
			}

			outFile, err := os.OpenFile(streams.Stderr.Path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
			if err != nil {
				return ctx, fmt.Errorf("failed to redirect stderr: %w", err)
			}

			defer func() {
				_ = outFile.Close()
			}()
			ctx.Streams.AdditionalStderr = append(ctx.Streams.AdditionalStderr, outFile)
			stderrRedirect = outFile
		}

		ctx, err := next(ctx)
		ctx.Streams.AdditionalStdout = slices.DeleteFunc(ctx.Streams.AdditionalStdout, func(w io.Writer) bool {
			return w == stdoutRedirect
		})
		ctx.Streams.AdditionalStderr = slices.DeleteFunc(ctx.Streams.AdditionalStderr, func(w io.Writer) bool {
			return w == stderrRedirect
		})

		ctx.Streams.Stdout = stdout
		ctx.Streams.Stderr = stderr

		return ctx, err
	}, nil
}
