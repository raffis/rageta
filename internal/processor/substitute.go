package processor

import (
	"fmt"
	"regexp"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

var substituteExpression = regexp.MustCompile(`(\$?\$)\(([^)($]+)\)`)

type Indexable interface {
	Index() map[string]string
}

func Subst(index Indexable, substitute ...any) error {
	vars := index.Index()

	for _, subst := range substitute {
		switch v := subst.(type) {
		case *string:
			result, err := parseExpression(*v, vars)
			if err != nil {
				return err
			}

			*v = result
		case []string:
			for k, val := range v {
				result, err := parseExpression(val, vars)
				if err != nil {
					return err
				}

				v[k] = result
			}
		case map[string]string:
			for k, val := range v {
				result, err := parseExpression(val, vars)
				if err != nil {
					return err
				}

				v[k] = result
			}
		case []v1beta1.Param:
			for k, val := range v {
				if param, err := substParam(&val, vars); err != nil {
					return err
				} else {
					v[k].Value = *param
				}
			}
		case *v1beta1.Param:
			if param, err := substParam(v, vars); err != nil {
				return err
			} else {
				v.Value = *param
			}
		default:
			return fmt.Errorf("type `%T` not substitutable", v)
		}
	}

	return nil
}

func substParam(param *v1beta1.Param, vars map[string]string) (*v1beta1.ParamValue, error) {
	switch param.Value.Type {
	case v1beta1.ParamTypeString:
		result, err := parseExpression(param.Value.StringVal, vars)

		if err != nil {
			return nil, err
		}

		err = param.Value.UnmarshalJSON([]byte(result))
		return &param.Value, err
	case v1beta1.ParamTypeArray:
		for k, val := range param.Value.ArrayVal {
			result, err := parseExpression(val, vars)
			if err != nil {
				return nil, err
			}

			param.Value.ArrayVal[k] = result
		}

		return &param.Value, nil
	}

	return nil, fmt.Errorf("unsupported param type `%s`", param.Value.Type)
}

func parseExpression(str string, vars map[string]string) (string, error) {
	var parseError error

	newStr := substituteExpression.ReplaceAllStringFunc(str, func(m string) string {
		parts := substituteExpression.FindStringSubmatch(m)
		if len(parts) != 3 {
			err := fmt.Errorf("invalid expression wrapper `%s` -- %#v", m, parts)
			if parseError == nil {
				parseError = err
			}
			return m
		}

		if parts[1] == `$$` {
			return fmt.Sprintf("$(%s)", parts[2])
		}

		if v, ok := vars[parts[2]]; ok {
			return v
		} else {
			return parts[0]
		}
	})

	return newStr, parseError
}
