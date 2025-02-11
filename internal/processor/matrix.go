package processor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithMatrix() ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Matrix == nil {
			return nil
		}

		return &Matrix{
			rawMatrix: spec.Matrix,
			stepName:  spec.Name,
		}
	}
}

type Matrix struct {
	rawMatrix map[string]json.RawMessage
	matrix    map[string]interface{}
	failFast  bool
	stepName  string
}

func (s *Matrix) UnmarshalJSON(b []byte) error {
	return json.Unmarshal(b, &s.matrix)
}

func (s *Matrix) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.rawMatrix)
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

		for matrixKey, matrix := range matrixes {
			copyContext := stepContext.DeepCopy()
			copyContext.Matrix = matrix
			copyContext.NamePrefix = PrefixName(matrixKey, copyContext.NamePrefix)

			go func(stepContext StepContext) {
				t, err := next(ctx, copyContext)
				results <- concurrentResult{t, err}
			}(copyContext)
		}

		var done int
	WAIT:
		for {
			select {
			case <-ctx.Done():
				return stepContext, nil
			case res := <-results:
				done++
				stepContext = stepContext.Merge(res.stepContext)
				if res.err != nil && AbortOnError(err) {
					errs = append(errs, res.err)

					if err != nil && s.failFast {
						break WAIT
					}
				}

				if done == len(matrixes) {
					break WAIT
				}
			}
		}

		if len(errs) > 0 {
			return stepContext, errors.Join(errs...)
		}

		return stepContext, nil
	}, nil
}

func (s *Matrix) build(matrix map[string]interface{}) (map[string]map[string]interface{}, error) {
	var keys []string
	for key := range matrix {
		keys = append(keys, key)
	}

	result := make(map[string]map[string]interface{})

	s.generateCombinations(matrix, keys, 0, make(map[string]interface{}), &result)

	return result, nil
}

func (s *Matrix) generateCombinations(mapData map[string]interface{}, keys []string, index int, currentCombination map[string]interface{}, result *map[string]map[string]interface{}) {
	// If we've added a value for each key, add the current combination to the result
	if index == len(keys) {
		// Create a copy of the current combination
		combinationCopy := make(map[string]interface{})
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

	// If the value is a slice, iterate through it
	if reflect.TypeOf(value).Kind() == reflect.Slice {
		// Convert the interface{} value to a slice of interfaces
		sliceValue := reflect.ValueOf(value)
		for i := 0; i < sliceValue.Len(); i++ {
			// Append the value to the current combination and recurse
			currentCombination[currentKey] = sliceValue.Index(i).Interface()
			s.generateCombinations(mapData, keys, index+1, currentCombination, result)
		}
	} else {
		// If it's not a slice, just use the value as the combination for this key
		currentCombination[currentKey] = value
		s.generateCombinations(mapData, keys, index+1, currentCombination, result)
	}
}
