package processor

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"

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
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		var stdoutRedirect, stderrRedirect io.Writer

		vars := []interface{}{}
		if s.streams.Stdout != nil {
			vars = append(vars, &s.streams.Stdout.Path)
		}
		if s.streams.Stderr != nil {
			vars = append(vars, &s.streams.Stderr.Path)
		}
		if s.streams.Stdin != nil {
			vars = append(vars, &s.streams.Stdin.Path)
		}

		if err := Subst(stepContext.ToV1Beta1(), vars...,
		); err != nil {
			return stepContext, err
		}

		if s.streams.Stdout != nil {
			if !s.tee {
				stepContext.Stdout = io.Discard
			}

			outFile, err := os.OpenFile(s.streams.Stdout.Path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
			if err != nil {
				return stepContext, fmt.Errorf("failed to redirect stdout: %w", err)
			}

			defer outFile.Close()
			stepContext.AdditionalStdout = append(stepContext.AdditionalStdout, outFile)
			stdoutRedirect = outFile
		}

		if s.streams.Stderr != nil {
			if !s.tee {
				stepContext.Stdout = io.Discard
			}

			outFile, err := os.OpenFile(s.streams.Stderr.Path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
			if err != nil {
				return stepContext, fmt.Errorf("failed to redirect stderr: %w", err)
			}

			defer outFile.Close()
			stepContext.AdditionalStderr = append(stepContext.AdditionalStderr, outFile)
			stderrRedirect = outFile
		}

		stepContext, err := next(ctx, stepContext)
		stepContext.AdditionalStdout = slices.DeleteFunc(stepContext.AdditionalStdout, func(w io.Writer) bool {
			return w == stdoutRedirect
		})
		stepContext.AdditionalStderr = slices.DeleteFunc(stepContext.AdditionalStderr, func(w io.Writer) bool {
			return w == stderrRedirect
		})

		return stepContext, err
	}, nil
}
