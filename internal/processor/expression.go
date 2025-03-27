package processor

import (
	"context"
	"fmt"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types/ref"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithExpression(celEnv *cel.Env) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Expression == "" {
			return nil
		}

		return &If{
			condition: spec.Expression,
			celEnv:    celEnv,
		}
	}
}

type Expression struct {
	expr   string
	celEnv *cel.Env
}

func (s *Expression) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	var expr cel.Program
	ast, issues := s.celEnv.Compile(s.expr)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("expression compilation `%s` failed: %w", s.expr, issues.Err())
	}

	prg, err := s.celEnv.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("expression ast `%s` failed: %w", s.expr, err)
	}

	expr = prg

	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		var out ref.Val

		out, _, err = expr.Eval(stepContext.RuntimeVars())
		if err != nil {
			return stepContext, fmt.Errorf("condition expression evaluation `%s` failed: %w", s.expr, err)
		}

		panic(out)

		return next(ctx, stepContext)
	}, nil
}
