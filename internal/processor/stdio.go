package processor

import (
	"context"
	"io"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

type OutputCloser func(err error) error
type OutputFactory func(ctx context.Context, stepContext StepContext, stepName string) (io.Writer, io.Writer, OutputCloser)

func WithStdio(outputFactory OutputFactory, withInternals bool) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		internalStep := spec.Run == nil

		if !withInternals && internalStep {
			return nil
		}

		stdio := &Stdio{
			stepName:      spec.Name,
			spec:          spec,
			outputFactory: outputFactory,
		}

		return stdio
	}
}

type Stdio struct {
	stepName      string
	spec          *v1beta1.Step
	outputFactory OutputFactory
}

func (s *Stdio) Bootstrap(pipelineCtx Pipeline, next Next) (Next, error) {
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
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
