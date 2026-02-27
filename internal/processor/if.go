package processor

import (
	"fmt"

	"github.com/google/cel-go/cel"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithIf(celEnv *cel.Env) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if len(spec.If) == 0 {
			return nil
		}

		return &If{
			celEnv:     celEnv,
			conditions: spec.If,
		}
	}
}

var ErrConditionFalse = &pipelineError{
	message:      "conditional step skipped",
	result:       "skipped-condition",
	abortOnError: false,
}

type If struct {
	celEnv     *cel.Env
	conditions []v1beta1.IfCondition
}

func (s *If) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	expr := make([]cel.Program, len(s.conditions))

	for i, condition := range s.conditions {
		if condition.CelExpression != nil {
			ast, issues := s.celEnv.Compile(*condition.CelExpression)
			if issues != nil && issues.Err() != nil {
				return nil, fmt.Errorf("if expression compilation `%s` failed: %w", *condition.CelExpression, issues.Err())
			}

			prg, err := s.celEnv.Program(ast)
			if err != nil {
				return nil, fmt.Errorf("if expression ast `%s` failed: %w", *condition.CelExpression, err)
			}

			expr[i] = prg
		}
	}

	return func(ctx StepContext) (StepContext, error) {
		vars := ctx.ToV1Beta1()
		for i, condition := range s.conditions {
			switch {
			case condition.CelExpression != nil:
				value, _, err := expr[i].ContextEval(ctx, map[string]any{
					"context": vars,
				})

				if err != nil {
					return ctx, fmt.Errorf("if expression evaluation `%s` failed: %w", *condition.CelExpression, err)
				}

				// if expression evaluates to false the next step is called in the pipeline without calling the
				// step handled by this wrapper first
				if !value.Value().(bool) {
					return ctx, ErrConditionFalse
				}
			default:
				return ctx, fmt.Errorf("invalid if condition given")
			}
		}

		return next(ctx)
	}, nil
}
