package processor

import (
	"context"
	"io"

	"github.com/raffis/rageta/internal/ioext"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

type OutputCloser func(err error) error
type OutputFactory func(ctx context.Context, stepContext StepContext, stepName string, stdin io.Reader, stdout, stderr io.Writer) (io.Reader, io.Writer, io.Writer, OutputCloser)

func WithStdio(tee bool, outputFactory OutputFactory, stdin io.Reader, stdout, stderr io.Writer) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		stdio := &Stdio{
			stepName:      spec.Name,
			tee:           tee,
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
	tee           bool
	spec          *v1beta1.Step
	stdin         io.Reader
	stdout        io.Writer
	stderr        io.Writer
	outputFactory OutputFactory
}

func (s *Stdio) Bootstrap(pipelineCtx Pipeline, next Next) (Next, error) {
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {

		_, stdout, stderr, close := s.outputFactory(ctx, stepContext, PrefixName(s.stepName, stepContext.NamePrefix), s.stdin, s.stdout, s.stderr)
		//var stdin io.Reader

		if stepContext.Stdout.Len() > 0 {
			w := stepContext.Stdout.Unpack()
			stepContext.Stdout = &ioext.MultiWriter{}
			stepContext.Stdout.Add(stdout)
			stepContext.Stdout.Add(w[1:]...)
		} else {
			stepContext.Stdout.Add(stdout)
		}

		if stepContext.Stderr.Len() > 0 {
			w := stepContext.Stderr.Unpack()
			stepContext.Stderr = &ioext.MultiWriter{}
			stepContext.Stderr.Add(stderr)
			stepContext.Stderr.Add(w[1:]...)
		} else {
			stepContext.Stderr.Add(stderr)
		}

		stepContext, err := next(ctx, stepContext)

		if err := close(err); err != nil {
			return stepContext, err
		}

		stepContext.Stderr.Remove(stderr)
		stepContext.Stdout.Remove(stdout)

		return stepContext, err
	}, nil
}
