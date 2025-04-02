package processor

import (
	"context"
	"fmt"

	"github.com/google/cel-go/cel"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithInput(celEnv *cel.Env) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if len(spec.Inputs) == 0 {
			return nil
		}

		return &Input{
			celEnv: celEnv,
			inputs: spec.Inputs,
		}
	}
}

type Input struct {
	celEnv *cel.Env
	inputs []v1beta1.InputParam
}

func (s *Input) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	expr := make(map[string]cel.Program)

	for _, input := range s.inputs {
		if input.CelExpression != nil {
			ast, issues := s.celEnv.Compile(*input.CelExpression)
			if issues != nil && issues.Err() != nil {
				return nil, fmt.Errorf("input expression compilation `%s` failed: %w", *input.CelExpression, issues.Err())
			}

			prg, err := s.celEnv.Program(ast)
			if err != nil {
				return nil, fmt.Errorf("input expression ast `%s` failed: %w", *input.CelExpression, err)
			}

			expr[input.Name] = prg
		}
	}

	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		vars := stepContext.ToV1Beta1()
		for _, input := range s.inputs {
			switch {
			case input.CelExpression != nil:
				value, _, err := expr[input.Name].ContextEval(ctx, map[string]any{
					"context": vars,
				})
				if err != nil {
					return stepContext, fmt.Errorf("input expression evaluation `%s` failed: %w", *input.CelExpression, err)
				}

				switch v := value.Value().(type) {
				case string:
					stepContext.Inputs[input.Name] = v1beta1.ParamValue{
						StringVal: v,
						Type:      v1beta1.ParamTypeString,
					}
				}

			case input.Default != nil:
				if _, ok := stepContext.Inputs[input.Name]; !ok {
					stepContext.Inputs[input.Name] = *input.Default
				}
			default:
				return stepContext, fmt.Errorf("invalid input param given `%s`", input.Name)
			}
		}

		return next(ctx, stepContext)
	}, nil
}
