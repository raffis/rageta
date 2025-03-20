package processor

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/google/cel-go/cel"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"google.golang.org/protobuf/types/known/structpb"
)

func WithSubstitute(celEnv *cel.Env, processors ...Bootstraper) ProcessorBuilder {
	var substitutions []Substitutor
	for _, processor := range processors {
		if v, ok := processor.(Substitutor); ok {
			substitutions = append(substitutions, v)
		}
	}

	return func(spec *v1beta1.Step) Bootstraper {
		if len(substitutions) == 0 {
			return nil
		}

		return &Substitute{
			celEnv:        celEnv,
			substitutions: substitutions,
			isMatrix:      spec.Matrix != nil && len(spec.Matrix.Params) > 0,
		}
	}
}

var (
	expression *regexp.Regexp
)

func init() {
	expression = regexp.MustCompile(`(\$\{\{([^}}]+)\}\})`)
}

type Substitutor interface {
	Substitute() []interface{}
}

type Substitute struct {
	celEnv        *cel.Env
	substitutions []Substitutor
	isMatrix      bool
}

func (s *Substitute) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		for _, substitution := range s.substitutions {
			//If the step has a matrix we only parse expressions from the matrix processor
			//This is due that any other steps could depend on matrix context which is not evaluated in a matrix parent
			_, isMatrixProcessor := substitution.(*Matrix)
			if s.isMatrix && len(stepContext.Matrix) == 0 && !isMatrixProcessor {
				continue
			}

			for _, subst := range substitution.Substitute() {
				switch v := subst.(type) {
				case *string:
					result, err := s.parseExpression(*v, stepContext.RuntimeVars())
					if err != nil {
						return stepContext, err
					}

					*v = result.(string)
				case []string:
					for k, val := range v {
						result, err := s.parseExpression(val, stepContext.RuntimeVars())
						if err != nil {
							return stepContext, err
						}

						v[k] = result.(string)
					}
				case map[string]string:
					for k, val := range v {
						result, err := s.parseExpression(val, stepContext.RuntimeVars())
						if err != nil {
							return stepContext, err
						}

						v[k] = result.(string)
					}
				case []v1beta1.Param:
					for k, val := range v {
						if param, err := s.substParam(&val, stepContext); err != nil {
							return stepContext, err
						} else {
							v[k].Value = *param
						}
					}
				case *v1beta1.Param:
					if param, err := s.substParam(v, stepContext); err != nil {
						return stepContext, err
					} else {
						v.Value = *param
					}
				default:
					return stepContext, fmt.Errorf("type `%T` not substitutable", v)
				}

			}
		}

		return next(ctx, stepContext)
	}, nil
}

func (s *Substitute) substParam(param *v1beta1.Param, stepContext StepContext) (*v1beta1.ParamValue, error) {
	toParam := func(v interface{}) (*v1beta1.ParamValue, error) {
		switch v := v.(type) {
		case string:
			return &v1beta1.ParamValue{
				Type:      v1beta1.ParamTypeString,
				StringVal: v,
			}, nil
		case []string:
			return &v1beta1.ParamValue{
				Type:     v1beta1.ParamTypeArray,
				ArrayVal: v,
			}, nil
		case *structpb.ListValue:
			typedMap := make([]string, len(v.Values))
			for i, v := range v.Values {
				if _, ok := v.Kind.(*structpb.Value_StringValue); ok {
					typedMap[i] = v.GetStringValue()
				} else {
					return nil, errors.New("arrays can only contain strings")
				}
			}

			return &v1beta1.ParamValue{
				Type:     v1beta1.ParamTypeArray,
				ArrayVal: typedMap,
			}, nil
		case []interface{}:
			typedMap := make([]string, len(v))
			for i, v := range v {
				if v, ok := v.(string); ok {
					typedMap[i] = v
				} else {
					return nil, errors.New("arrays can only contain strings")
				}
			}

			return &v1beta1.ParamValue{
				Type:     v1beta1.ParamTypeArray,
				ArrayVal: typedMap,
			}, nil
		default:
			return nil, errors.New("can not convert result to param")
		}
	}

	switch param.Value.Type {
	case v1beta1.ParamTypeString:
		result, err := s.parseExpression(param.Value.StringVal, stepContext.RuntimeVars())
		if err != nil {
			return nil, err
		}

		return toParam(result)

	case v1beta1.ParamTypeArray:
		for k, val := range param.Value.ArrayVal {
			result, err := s.parseExpression(val, stepContext.RuntimeVars())
			if err != nil {
				return nil, err
			}

			param.Value.ArrayVal[k] = result.(string)
		}
	}

	return nil, errors.New("unsupported param type")
}

func (s *Substitute) parseExpression(str string, vars interface{}) (interface{}, error) {
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
