package processor

import (
	"slices"

	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithStdioRedirect(tee bool) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Run == nil || spec.Run.Streams == nil {
			return nil
		}

		stdio := &StdioRedirect{
			streams: spec.Run.Streams,
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

		originStdoutPaths := slices.Clone(ctx.AdditionalStdoutPaths)
		originStderrPaths := slices.Clone(ctx.AdditionalStderrPaths)

		if s.streams.Stdout != nil {
			ctx.AdditionalStdoutPaths = append(ctx.AdditionalStdoutPaths, s.streams.Stdout.Path)
		}

		if s.streams.Stderr != nil {
			ctx.AdditionalStderrPaths = append(ctx.AdditionalStderrPaths, s.streams.Stderr.Path)
		}

		if s.streams.Stdin != nil {
			ctx.StdinPath = s.streams.Stdin.Path
		}

		ctx, err := next(ctx)
		ctx.AdditionalStdoutPaths = originStdoutPaths
		ctx.AdditionalStderrPaths = originStderrPaths

		return ctx, err
	}, nil
}
