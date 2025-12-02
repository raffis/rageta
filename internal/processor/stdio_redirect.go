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

		stdout := ctx.Stdout
		stderr := ctx.Stderr

		if streams.Stdout != nil {
			if !s.tee {
				ctx.Stdout = io.Discard
			}

			outFile, err := os.OpenFile(streams.Stdout.Path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
			if err != nil {
				return ctx, fmt.Errorf("failed to redirect stdout: %w", err)
			}

			defer func() {
				_ = outFile.Close()
			}()
			ctx.AdditionalStdout = append(ctx.AdditionalStdout, outFile)
			stdoutRedirect = outFile
		}

		if streams.Stderr != nil {
			if !s.tee {
				ctx.Stdout = io.Discard
			}

			outFile, err := os.OpenFile(streams.Stderr.Path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
			if err != nil {
				return ctx, fmt.Errorf("failed to redirect stderr: %w", err)
			}

			defer func() {
				_ = outFile.Close()
			}()
			ctx.AdditionalStderr = append(ctx.AdditionalStderr, outFile)
			stderrRedirect = outFile
		}

		ctx, err := next(ctx)
		ctx.AdditionalStdout = slices.DeleteFunc(ctx.AdditionalStdout, func(w io.Writer) bool {
			return w == stdoutRedirect
		})
		ctx.AdditionalStderr = slices.DeleteFunc(ctx.AdditionalStderr, func(w io.Writer) bool {
			return w == stderrRedirect
		})

		ctx.Stdout = stdout
		ctx.Stderr = stderr

		return ctx, err
	}, nil
}
