package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/google/cel-go/cel"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithExpressionParser(celEnv *cel.Env, processors ...Bootstraper) ProcessorBuilder {
	var lateVarBinders []lateVarBinder
	for _, processor := range processors {
		if v, ok := processor.(lateVarBinder); ok {
			lateVarBinders = append(lateVarBinders, v)
		}
	}

	return func(spec *v1beta1.Step) Bootstraper {
		if len(lateVarBinders) == 0 {
			return nil
		}

		return &ExpressionParser{
			celEnv:         celEnv,
			lateVarBinders: lateVarBinders,
		}
	}
}

var (
	expression *regexp.Regexp
)

func init() {
	expression = regexp.MustCompile(`(\$\{\{([^}}]+)\}\})`)
}

type lateVarBinder interface {
	json.Marshaler
	json.Unmarshaler
}

type ExpressionParser struct {
	celEnv         *cel.Env
	lateVarBinders []lateVarBinder
}

func (s *ExpressionParser) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		for _, lateVarBinder := range s.lateVarBinders {
			b, err := lateVarBinder.MarshalJSON()
			if err != nil {
				return stepContext, fmt.Errorf("failed to marshal processor to json: %w", err)
			}

			str, err := s.parseExpression(string(b), stepContext.RuntimeVars())
			if err != nil {
				return stepContext, fmt.Errorf("parse expressions `%s` failed: %w", string(b), err)
			}

			if err = lateVarBinder.UnmarshalJSON([]byte(str)); err != nil {
				return stepContext, fmt.Errorf("failed to unmarshal json `%s` to processor failed: %w", str, err)
			}
		}

		return next(ctx, stepContext)
	}, nil
}

func (s *ExpressionParser) parseExpression(str string, vars interface{}) (string, error) {
	var parseError error
	return expression.ReplaceAllStringFunc(str, func(m string) string {
		parts := expression.FindStringSubmatch(m)

		if len(parts) != 3 {
			err := fmt.Errorf("invalid expression wrapper %s", m)
			if parseError == nil {
				parseError = err
			}
			return m
		}

		ast, issues := s.celEnv.Compile(parts[2])
		if issues != nil && issues.Err() != nil {
			err := fmt.Errorf("expression compilation %s failed: %w", parts[2], issues.Err())
			if parseError == nil {
				parseError = err
			}

			return m
		}

		prg, err := s.celEnv.Program(ast)
		if err != nil {
			err := fmt.Errorf("expression ast %s failed: %w", parts[2], err)
			if parseError == nil {
				parseError = err
			}

			return m
		}

		val, _, err := prg.Eval(vars)

		if err != nil {
			err := fmt.Errorf("expression evaluation %s failed: %w", parts[2], err)
			if parseError == nil {
				parseError = err
			}

			return m
		}

		v, ok := val.Value().(string)
		if !ok {
			parseError = fmt.Errorf("expression result %s failed, expected string", parts[2])
		} else {
			return v
		}

		return ""
	}), parseError
}
