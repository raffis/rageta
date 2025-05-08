package processor

import (
	"context"
	"io"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

type OutputCloser func(err error) error
type OutputFactory func(ctx context.Context, stepContext StepContext, stepName string) (io.Writer, io.Writer, OutputCloser)

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
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		if _, ok := stepContext.Tags["inherit"]; ok && !s.decouple {
			return next(ctx, stepContext)
		}

		stdout, stderr, close := s.outputFactory(ctx, stepContext, suffixName(s.stepName, stepContext.NamePrefix))

		if stepContext.Stdout != io.Discard {
			stepContext.Stdout = stdout
		}

		if stepContext.Stderr != io.Discard {
			stepContext.Stderr = stderr
		}

		stepContext, err := next(ctx, stepContext)

		if err := close(err); err != nil {
			return stepContext, err
		}

		stepContext.Stderr = nil
		stepContext.Stdout = nil

		return stepContext, err
	}, nil
}
