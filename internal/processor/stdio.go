package processor

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithStdio() ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Streams == nil {
			return nil
		}

		stdio := &Stdio{
			spec:    spec,
			streams: spec.Streams,
		}

		return stdio
	}
}

type Stdio struct {
	spec    *v1beta1.Step
	streams *v1beta1.Streams
}

func (s *Stdio) Substitute() []*Substitute {
	var vals []*Substitute
	if s.streams == nil {
		return vals
	}

	if s.streams.Stdout != nil {

		vals = append(vals, &Substitute{
			v: s.streams.Stdout.Path,
			f: func(v interface{}) {
				fmt.Printf("\nSUBS %#v -- \n\n", v.(string))

				s.streams.Stdout.Path = v.(string)
			},
		})
	}
	if s.streams.Stderr != nil {
		vals = append(vals, &Substitute{
			v: s.streams.Stderr.Path,
			f: func(v interface{}) {
				s.streams.Stderr.Path = v.(string)
			},
		})
	}
	if s.streams.Stdin != nil {
		vals = append(vals, &Substitute{
			v: s.streams.Stdin.Path,
			f: func(v interface{}) {
				s.streams.Stdin.Path = v.(string)
			},
		})
	}

	return vals
}

func (s *Stdio) Bootstrap(pipelineCtx Pipeline, next Next) (Next, error) {
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		var stdout, stderr io.Writer

		if s.streams.Stdout != nil {
			fmt.Printf("\n\nSTDOUT %#v -- %#v\n\n", s.spec.Name, stepContext.Output)

			outFile, err := os.OpenFile(s.streams.Stdout.Path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
			if err != nil {
				return stepContext, fmt.Errorf("failed to redirect stdout: %w", err)
			}

			defer outFile.Close()
			stepContext.Stdout.Add(outFile)
			stdout = outFile
		}

		if s.streams.Stderr != nil {
			outFile, err := os.OpenFile(s.streams.Stderr.Path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
			if err != nil {
				return stepContext, fmt.Errorf("failed to redirect stderr: %w", err)
			}

			defer outFile.Close()
			stepContext.Stdout.Add(outFile)
			stderr = outFile
		}

		stepContext, err := next(ctx, stepContext)
		stepContext.Stdout.Remove(stdout)
		stepContext.Stderr.Remove(stderr)

		return stepContext, err
	}, nil
}
