package processor

import (
	"fmt"
	"io"
	"os"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithOutputVars() ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if len(spec.Outputs) == 0 {
			return nil
		}

		return &OutputVars{
			stepName: spec.Name,
			outputs:  spec.Outputs,
		}
	}
}

type OutputVars struct {
	stepName string
	outputs  []v1beta1.StepOutputParam
}

func (s *OutputVars) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		outputs := make(map[string]*os.File, len(s.outputs))

		for _, output := range s.outputs {
			outputTmp, err := os.CreateTemp(ctx.Dir, "output")
			if err != nil {
				return ctx, err
			}

			defer func(f *os.File) {
				_ = f.Close()
				_ = os.Remove(f.Name())
			}(outputTmp)

			ctx.Outputs = append(ctx.Outputs, OutputParam{
				Name: output.Name,
				Path: outputTmp.Name(),
			})

			outputs[output.Name] = outputTmp
		}

		ctx, err := next(ctx)
		if err != nil {
			return ctx, err
		}

		for name, output := range outputs {
			_ = output.Sync()
			b, err := io.ReadAll(output)
			if err != nil {
				return ctx, err
			}

			value := v1beta1.ParamValue{}

			if err := value.UnmarshalJSON(b); err != nil {
				return ctx, fmt.Errorf("param output failed: %w", err)
			}

			ctx.OutputVars[name] = value
		}

		return ctx, err

	}, nil
}
