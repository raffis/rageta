package processor

import (
	"io"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

type OutputCloser func(err error) error
type OutputFactory func(ctx StepContext, stepName, short string) (io.Writer, io.Writer, OutputCloser)

func WithOutput(outputFactory OutputFactory, withInternals, decouple bool) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		internalStep := spec.Run == nil && spec.Inherit == nil

		if !withInternals && internalStep {
			return nil
		}

		stdio := &Output{
			stepName:      spec.Name,
			short:         spec.Short,
			spec:          spec,
			outputFactory: outputFactory,
			decouple:      decouple,
		}

		return stdio
	}
}

type Output struct {
	stepName      string
	short         string
	spec          *v1beta1.Step
	outputFactory OutputFactory
	decouple      bool
}

type StreamsContext struct {
	Stdin            io.Reader
	Stdout           io.Writer
	Stderr           io.Writer
	AdditionalStdout []io.Writer
	AdditionalStderr []io.Writer
}

func (s *Output) Bootstrap(pipelineCtx Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		if ctx.Tags.Has("pipeline") && !s.decouple {
			return next(ctx)
		}

		stdout, stderr, close := s.outputFactory(ctx, s.stepName, s.short)

		if ctx.Streams.Stdout != io.Discard {
			ctx.Streams.Stdout = stdout
		}

		if ctx.Streams.Stderr != io.Discard {
			ctx.Streams.Stderr = stderr
		}

		ctx, err := next(ctx)
		if err := close(err); err != nil {
			return ctx, err
		}

		ctx.Streams.Stderr = nil
		ctx.Streams.Stdout = nil

		return ctx, err
	}, nil
}
