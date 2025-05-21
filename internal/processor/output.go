package processor

import (
	"io"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

type OutputCloser func(err error) error
type OutputFactory func(ctx StepContext, stepName string) (io.Writer, io.Writer, OutputCloser)

func WithOutput(outputFactory OutputFactory, withInternals, decouple bool) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		internalStep := spec.Run == nil && spec.Inherit == nil

		if !withInternals && internalStep {
			return nil
		}

		stdio := &Output{
			stepName:      spec.Name,
			spec:          spec,
			outputFactory: outputFactory,
			decouple:      decouple,
		}

		return stdio
	}
}

type Output struct {
	stepName      string
	spec          *v1beta1.Step
	outputFactory OutputFactory
	decouple      bool
}

func (s *Output) Bootstrap(pipelineCtx Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		if ctx.HasTag("inherit") && !s.decouple {
			return next(ctx)
		}

		stdout, stderr, close := s.outputFactory(ctx, suffixName(s.stepName, ctx.NamePrefix))

		if ctx.Stdout != io.Discard {
			ctx.Stdout = stdout
		}

		if ctx.Stderr != io.Discard {
			ctx.Stderr = stderr
		}

		ctx, err := next(ctx)

		if err := close(err); err != nil {
			return ctx, err
		}

		ctx.Stderr = nil
		ctx.Stdout = nil

		return ctx, err
	}, nil
}
