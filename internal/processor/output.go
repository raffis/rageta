package processor

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithOutput() ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if len(spec.Outputs) == 0 {
			return nil
		}

		return &Output{
			stepName: spec.Name,
			outputs:  spec.Outputs,
		}
	}
}

type Output struct {
	stepName string
	outputs  []v1beta1.StepOutputParam
}

func (s *Output) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		outputs := make(map[string]*os.File, len(s.outputs))

		for _, output := range s.outputs {
			outputTmp, err := os.CreateTemp(stepContext.TmpDir(), "output")
			if err != nil {
				return stepContext, err
			}

			defer outputTmp.Close()
			defer os.Remove(outputTmp.Name())

			stepContext.Outputs = append(stepContext.Outputs, OutputParam{
				Name: output.Name,
				Path: outputTmp.Name(),
			})

			outputs[output.Name] = outputTmp
		}

		stepContext, err := next(ctx, stepContext)
		if err != nil {
			return stepContext, err
		}

		for name, output := range outputs {
			output.Sync()
			b, err := io.ReadAll(output)
			if err != nil {
				return stepContext, err
			}

			value := v1beta1.ParamValue{}

			if err := value.UnmarshalJSON(b); err != nil {
				return stepContext, fmt.Errorf("param output failed: %w", err)
			}

			stepContext.Steps[s.stepName].Outputs[name] = value
		}

		return stepContext, err

	}, nil
}
