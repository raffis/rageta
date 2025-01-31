package processor

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types/ref"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithIf(celEnv *cel.Env) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.If == "" {
			return nil
		}

		return &If{
			condition: spec.If,
			celEnv:    celEnv,
		}
	}
}

type If struct {
	condition string
	celEnv    *cel.Env
}

var ErrConditionFalse = errors.New("conditional step skipped")

func (s *If) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	var ifCondition cel.Program
	ast, issues := s.celEnv.Compile(s.condition)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("expression compilation `%s` failed: %w", s.condition, issues.Err())
	}

	prg, err := s.celEnv.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("expression ast `%s` failed: %w", s.condition, err)
	}

	ifCondition = prg

	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		var out ref.Val

		out, _, err = ifCondition.Eval(stepContext.RuntimeVars())
		if err != nil {
			return stepContext, fmt.Errorf("condition expression evaluation `%s` failed: %w", s.condition, err)
		}

		// if expression evaluates to false the next step is called in the pipeline without calling the
		// step handled by this wrapper first
		if !out.Value().(bool) {
			return stepContext, ErrConditionFalse
		}

		return next(ctx, stepContext)
	}, nil
}
