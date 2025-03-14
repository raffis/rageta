package processor

import (
	"github.com/google/cel-go/cel"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithExpression(celEnv *cel.Env) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Expression == nil {
			return nil
		}

		return &Expression{
			script: spec.Expression.Script,
			celEnv: celEnv,
		}
	}
}

type Expression struct {
	script string
	celEnv *cel.Env
}

func (s *Expression) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	/*var ExpressionCondition cel.Program
	ast, issues := s.celEnv.Compile(s.script)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("expression compilation `%s` failed: %w", s.script, issues.Err())
	}

	prg, err := s.celEnv.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("expression ast `%s` failed: %w", s.script, err)
	}

	ExpressionCondition = prg

	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		//var out ref.Val

		//	out, _, err = ExpressionCondition.Eval(stepContext.RuntimeVars())
		if err != nil {
			return stepContext, fmt.Errorf("expression evaluation `%s` failed: %w", s.script, err)
		}

		return next(ctx, stepContext)
	}, nil*/
	return nil, nil
}
