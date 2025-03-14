package processor

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"google.golang.org/protobuf/types/known/structpb"
)

func WithMatrix() ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Matrix == nil {
			return nil
		}

		return &Matrix{
			matrix:   spec.Matrix.Params,
			failFast: spec.Matrix.FailFast,
			stepName: spec.Name,
		}
	}
}

type Matrix struct {
	matrix   []v1beta1.Param
	failFast bool
	stepName string
}

type Substitute struct {
	v interface{}
	f func(v interface{})
}

func (s *Matrix) Substitute() []*Substitute {
	var vals []*Substitute
	for k, param := range s.matrix {
		var v interface{}
		switch param.Value.Type {
		case v1beta1.ParamTypeString:
			v = param.Value.StringVal
		case v1beta1.ParamTypeArray:
			v = param.Value.ArrayVal
		}

		vals = append(vals, &Substitute{
			v: v,
			f: func(v interface{}) {
				switch v := v.(type) {
				case string:
					s.matrix[k].Value = v1beta1.ParamValue{
						Type:      v1beta1.ParamTypeString,
						StringVal: v,
					}
				case []string:
					s.matrix[k].Value = v1beta1.ParamValue{
						Type:     v1beta1.ParamTypeArray,
						ArrayVal: v,
					}
				case *structpb.ListValue:
					typedMap := make([]string, len(v.Values))
					for i, v := range v.Values {
						if _, ok := v.Kind.(*structpb.Value_StringValue); ok {
							typedMap[i] = v.GetStringValue()
						} else {
							panic("string map")
						}
					}

					s.matrix[k].Value = v1beta1.ParamValue{
						Type:     v1beta1.ParamTypeArray,
						ArrayVal: typedMap,
					}
				case []interface{}:
					typedMap := make([]string, len(v))
					for i, v := range v {
						if v, ok := v.(string); ok {
							typedMap[i] = v
						} else {
							panic("string map")
						}
					}

					s.matrix[k].Value = v1beta1.ParamValue{
						Type:     v1beta1.ParamTypeArray,
						ArrayVal: typedMap,
					}
				default:
					panic("x")
				}
			},
		})

	}

	return vals
}

func (s *Matrix) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		//Only proceed if we are not in a a matrix child context
		if len(stepContext.Matrix) > 0 {
			return next(ctx, stepContext)
		}

		step, err := pipeline.Step(s.stepName)
		if err != nil {
			return stepContext, err
		}

		next, err := step.Entrypoint()
		if err != nil {
			return stepContext, err
		}

		matrixes, err := s.build(s.matrix)
		if err != nil {
			return stepContext, err
		}

		results := make(chan concurrentResult)
		var errs []error

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		for matrixKey, matrix := range matrixes {
			copyContext := stepContext.DeepCopy()
			copyContext.Matrix = matrix

			hasher := sha1.New()
			hasher.Write([]byte(matrixKey))
			b := hasher.Sum(nil)

			copyContext.NamePrefix = PrefixName(hex.EncodeToString(b)[:6], copyContext.NamePrefix)

			go func(copyContext StepContext) {
				t, err := next(ctx, copyContext)
				results <- concurrentResult{t, err}
			}(copyContext)
		}

		var done int
	WAIT:
		for res := range results {
			done++
			stepContext = stepContext.Merge(res.stepContext)
			if res.err != nil && AbortOnError(res.err) {
				errs = append(errs, res.err)

				if s.failFast {
					cancel()
				}
			}

			if done == len(matrixes) {
				break WAIT
			}
		}

		if len(errs) > 0 {
			return stepContext, errors.Join(errs...)
		}

		return stepContext, nil
	}, nil
}

func (s *Matrix) build(matrix []v1beta1.Param) (map[string]map[string]string, error) {
	var keys []string
	mapData := make(map[string]v1beta1.ParamValue)

	for _, param := range matrix {
		keys = append(keys, param.Name)
		mapData[param.Name] = param.Value
	}

	result := make(map[string]map[string]string)

	s.generateCombinations(mapData, keys, 0, make(map[string]string), &result)

	return result, nil
}

func (s *Matrix) generateCombinations(mapData map[string]v1beta1.ParamValue, keys []string, index int, currentCombination map[string]string, result *map[string]map[string]string) {
	// If we've added a value for each key, add the current combination to the result
	if index == len(keys) {
		// Create a copy of the current combination
		combinationCopy := make(map[string]string)
		for k, v := range currentCombination {
			combinationCopy[k] = v
		}

		// Create a unique key by concatenating values in currentCombination
		var combinationValues []string
		for _, key := range keys {
			val := currentCombination[key]
			// Handle different types (e.g., slices)
			if reflect.TypeOf(val).Kind() == reflect.Slice {
				// If it's a slice, join all its elements with a delimiter
				sliceVal := reflect.ValueOf(val)
				for i := 0; i < sliceVal.Len(); i++ {
					combinationValues = append(combinationValues, fmt.Sprintf("%v", sliceVal.Index(i).Interface()))
				}
			} else {
				combinationValues = append(combinationValues, fmt.Sprintf("%v", val))
			}
		}
		// Join the values using "-" as a delimiter to form the unique key
		uniqueKey := strings.Join(combinationValues, "-")
		(*result)[uniqueKey] = combinationCopy
		return
	}

	// Get the current key and its corresponding value
	currentKey := keys[index]
	value := mapData[currentKey]

	switch value.Type {
	case v1beta1.ParamTypeString:
		currentCombination[currentKey] = value.StringVal
		s.generateCombinations(mapData, keys, index+1, currentCombination, result)
	case v1beta1.ParamTypeArray:
		for _, v := range value.ArrayVal {
			currentCombination[currentKey] = v
			s.generateCombinations(mapData, keys, index+1, currentCombination, result)
		}
	}
}
