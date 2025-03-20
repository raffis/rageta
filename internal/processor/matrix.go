package processor

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/alitto/pond/v2"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithMatrix(pool pond.Pool) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Matrix == nil || pool == nil {
			return nil
		}

		return &Matrix{
			matrix:   spec.Matrix.Params,
			failFast: spec.Matrix.FailFast,
			stepName: spec.Name,
			pool:     pool,
		}
	}
}

type Matrix struct {
	matrix   []v1beta1.Param
	failFast bool
	stepName string
	pool     pond.Pool
}

func (s *Matrix) Substitute() []interface{} {
	return []interface{}{s.matrix}
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

		fmt.Printf("bBBBBBBBBBBBBBBBBBB %#v\n", s.matrix)

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

			s.pool.Go(func() {
				t, err := next(ctx, copyContext)
				results <- concurrentResult{t, err}
			})
		}

		var done int
	WAIT:
		for res := range results {
			done++

			for k, step := range res.stepContext.Steps {
				fmt.Printf("range %s (%s) step.Outputs %#v \n", k, s.stepName, step.Outputs)
				for paramKey, paramValue := range step.Outputs {
					fmt.Printf("va %#v \n", paramValue)
					if paramValue.Type == v1beta1.ParamTypeString {
						var param v1beta1.ParamValue
						if _, ok := stepContext.Steps[s.stepName].Outputs[paramKey]; !ok {
							param = v1beta1.ParamValue{
								Type:     v1beta1.ParamTypeArray,
								ArrayVal: []string{paramValue.StringVal},
							}
						} else {
							param = stepContext.Steps[s.stepName].Outputs[paramKey]
							param.ArrayVal = append(param.ArrayVal, paramValue.StringVal)
						}

						fmt.Printf("\n\nmat steps)(%s): %#v\n\n", s.stepName, stepContext.Steps[s.stepName])

						stepContext.Steps[s.stepName].Outputs[paramKey] = param
					}
				}
			}

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
