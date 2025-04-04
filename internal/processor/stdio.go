package processor

import (
	"context"
	"io"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

type OutputCloser func(err error) error
type OutputFactory func(ctx context.Context, stepContext StepContext, stepName string, stdin io.Reader, stdout, stderr io.Writer) (io.Reader, io.Writer, io.Writer, OutputCloser)

func WithStdio(outputFactory OutputFactory, stdin io.Reader, stdout, stderr io.Writer) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		stdio := &Stdio{
			stepName:      spec.Name,
			spec:          spec,
			outputFactory: outputFactory,
			stdin:         stdin,
			stdout:        stdout,
			stderr:        stderr,
		}

		return stdio
	}
}

type Stdio struct {
	stepName      string
	spec          *v1beta1.Step
	stdin         io.Reader
	stdout        io.Writer
	stderr        io.Writer
	outputFactory OutputFactory
}

func (s *Stdio) Bootstrap(pipelineCtx Pipeline, next Next) (Next, error) {
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		_, stdout, stderr, close := s.outputFactory(ctx, stepContext, PrefixName(s.stepName, stepContext.NamePrefix), s.stdin, s.stdout, s.stderr)

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
