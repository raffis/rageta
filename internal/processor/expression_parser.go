package processor

import (
	"context"
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
			isMatrix:       spec.Matrix != nil && len(spec.Matrix.Params) > 0,
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
	Substitute() []*Substitute
}

type ExpressionParser struct {
	celEnv         *cel.Env
	lateVarBinders []lateVarBinder
	isMatrix       bool
}

func (s *ExpressionParser) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		for _, lateVarBinder := range s.lateVarBinders {
			//If the step has a matrix we only parse expressions from the matrix processor
			//This is due that any other steps could depend on matrix context which is not evaluated in a matrix parent
			_, isMatrixProcessor := lateVarBinder.(*Matrix)
			if s.isMatrix && len(stepContext.Matrix) == 0 && !isMatrixProcessor {
				continue
			}

			for _, v := range lateVarBinder.Substitute() {

				switch subst := v.v.(type) {
				case string:
					result, err := s.parseExpression(subst, stepContext.RuntimeVars())
					if err != nil {
						return stepContext, err
					}

					v.f(result)
				case []string:
					for k, val := range subst {
						result, err := s.parseExpression(val, stepContext.RuntimeVars())
						if err != nil {
							return stepContext, err
						}

						subst[k] = result.(string)
					}

					v.f(subst)
				case map[string]string:
					for k, val := range subst {
						result, err := s.parseExpression(val, stepContext.RuntimeVars())
						if err != nil {
							return stepContext, err
						}

						subst[k] = result.(string)
					}

					v.f(subst)
				}
			}
		}

		return next(ctx, stepContext)
	}, nil
}

func (s *ExpressionParser) parseExpression(str string, vars interface{}) (interface{}, error) {
	var parseError error
	var result interface{}

	newStr := expression.ReplaceAllStringFunc(str, func(m string) string {
		parts := expression.FindStringSubmatch(m)

		if len(parts) != 3 {
			err := fmt.Errorf("invalid expression wrapper %s -- %#v", m, parts)
			if parseError == nil {
				parseError = err
			}
			return m
		}

		ast, issues := s.celEnv.Compile(parts[2])
		if issues != nil && issues.Err() != nil {
			err := fmt.Errorf("expression compilation %s failed: %w -- %#v", parts[2], issues.Err(), parts)
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
			err := fmt.Errorf("expression evaluation `%s` failed: %w", parts[2], err)
			if parseError == nil {
				parseError = err
			}

			return m
		}

		v := val.Value()
		if str == parts[0] {
			result = v
			return m
		}

		result, ok := v.(string)
		if !ok {
			parseError = fmt.Errorf("expression result %s failed, expected string", parts[2])
		} else {
			return result
		}

		return ""
	})

	if result != nil {
		return result, parseError
	}

	return newStr, parseError
}
