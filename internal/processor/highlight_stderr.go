package processor

import (
	"io"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithHighlightStderr() ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		highlightStderr := &HighlightStderr{}

		return highlightStderr
	}
}

type HighlightStderr struct {
}

func (s *HighlightStderr) Bootstrap(pipelineCtx Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		originStderr := ctx.Stderr
		if ctx.Stderr != io.Discard && ctx.Stderr != nil {
			ctx.Stderr = &RedWriter{W: originStderr}
		}

		ctx, err := next(ctx)
		ctx.Stderr = originStderr
		return ctx, err
	}, nil
}

type RedWriter struct {
	W io.Writer
}

func (r RedWriter) Write(p []byte) (int, error) {
	// ANSI red start + text + reset
	colored := append([]byte("\033[31m"), p...)
	colored = append(colored, []byte("\033[0m")...)

	_, err := r.W.Write(colored)
	if err != nil {
		return 0, err
	}

	// Return original length to satisfy io.Writer contract
	return len(p), nil
}
