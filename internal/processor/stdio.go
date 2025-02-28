package processor

import (
	"context"
	"encoding/json"
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
			streams: spec.Streams,
		}

		return stdio
	}
}

type Stdio struct {
	streams *v1beta1.Streams
}

func (s *Stdio) UnmarshalJSON(b []byte) error {
	return json.Unmarshal(b, &s.streams)
}

func (s *Stdio) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.streams)
}

func (s *Stdio) Bootstrap(pipelineCtx Pipeline, next Next) (Next, error) {
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		var stdout, stderr io.Writer

		if s.streams.Stdout != nil {
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
