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
	outputs  []v1beta1.Param
}

func (s *Output) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		for _, output := range s.outputs {
			outputTmp, err := os.CreateTemp(stepContext.TmpDir(), "output")
			if err != nil {
				return stepContext, err
			}

			defer outputTmp.Close()
			defer os.Remove(outputTmp.Name())

			stepContext.Outputs = append(stepContext.Outputs, OutputParam{
				Name: output.Name,
				file: outputTmp,
			})
		}
		fmt.Printf("\n \n ==> step tp(%s) %#v\n\n", s.stepName, stepContext.Outputs)

		stepContext, err := next(ctx, stepContext)
		fmt.Printf("\n \n ==> step from(%s) %#v\n\n", s.stepName, stepContext.Outputs)
		if err != nil {
			return stepContext, err
		}

		for i, output := range stepContext.Outputs {
			output.file.Sync()

			b, err := io.ReadAll(output.file)
			if err != nil {
				return stepContext, err
			}

			value := v1beta1.ParamValue{}

			fmt.Printf("\n \n ==> UNMARH %#v\n\n", string(b))

			if err := value.UnmarshalJSON(b); err != nil {
				return stepContext, fmt.Errorf("param output failed: %w", err)
			}
			fmt.Printf("\n \n ==> UNMARH DONE %#v\n\n", string(b))

			stepContext.Steps[s.stepName].Outputs[output.Name] = value

			stepContext.Outputs = append(stepContext.Outputs[:i], stepContext.Outputs[i+1:]...)

		}

		return stepContext, err

	}, nil
}
