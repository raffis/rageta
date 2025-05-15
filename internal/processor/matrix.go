package processor

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"maps"

	"github.com/alitto/pond/v2"
	"github.com/raffis/rageta/internal/styles"
	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithMatrix(pool pond.Pool) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Matrix == nil || len(spec.Matrix.Params) == 0 || pool == nil {
			return nil
		}

		return &Matrix{
			matrix:        spec.Matrix.Params,
			include:       spec.Matrix.Include,
			failFast:      spec.Matrix.FailFast,
			stepName:      spec.Name,
			pool:          pool,
			maxConcurrent: spec.Matrix.MaxConcurrent,
		}
	}
}

type Matrix struct {
	matrix        []v1beta1.Param
	include       []v1beta1.IncludeParam
	failFast      bool
	stepName      string
	pool          pond.Pool
	maxConcurrent int
}

var ErrEmptyMatrix = errors.New("empty matrix")

func (s *Matrix) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		subst := []any{s.matrix}
		for _, group := range s.include {
			subst = append(subst, group.Params)
		}

		if err := substitute.Substitute(stepContext.ToV1Beta1(),
			subst...,
		); err != nil {
			return stepContext, fmt.Errorf("substitution failed: %w", err)
		}

		matrixes, err := s.build(s.matrix)
		if err != nil {
			return stepContext, err
		}

		if len(matrixes) == 0 {
			return stepContext, ErrEmptyMatrix
		}

		results := make(chan concurrentResult)
		var errs []error

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		pool := s.pool
		if s.maxConcurrent > 0 {
			pool = s.pool.NewSubpool(s.maxConcurrent)
		}

		for matrixKey, matrix := range matrixes {
			copyContext := stepContext.DeepCopy()
			for paramKey, paramValue := range matrix {
				copyContext.Tags[fmt.Sprintf("matrix/%s", paramKey)] = Tag{
					Key:   fmt.Sprintf("matrix/%s", paramKey),
					Value: paramValue,
					Color: styles.RandHEXColor(0, 255),
				}
			}

			s.combineIncludes(matrix, s.include)
			copyContext.Matrix = matrix

			hasher := sha1.New()
			hasher.Write([]byte(matrixKey))
			b := hasher.Sum(nil)

			if copyContext.NamePrefix == "" {
				copyContext.NamePrefix = hex.EncodeToString(b)[:6]
			} else {
				copyContext.NamePrefix = suffixName(copyContext.NamePrefix, hex.EncodeToString(b)[:6])
			}

			pool.Go(func() {
				t, err := next(ctx, copyContext)
				results <- concurrentResult{t, err}
			})
		}

		var done int
	WAIT:
		for res := range results {
			done++

			for stepName, step := range res.stepContext.Steps {
				//Copy matrix step result to current context
				if _, ok := stepContext.Steps[stepName]; ok {
					continue
				}

				stepContext.Steps[suffixName(stepName, res.stepContext.NamePrefix)] = step

				//Unify matrix outputs into an array output for the current step
				for paramKey, paramValue := range step.OutputVars {
					var param v1beta1.ParamValue

					if val, ok := stepContext.OutputVars[paramKey]; !ok {
						param = v1beta1.ParamValue{
							Type: v1beta1.ParamTypeArray,
						}
					} else {
						param = val
					}

					if paramValue.Type == v1beta1.ParamTypeString && paramValue.StringVal != "" {
						param.ArrayVal = append(param.ArrayVal, paramValue.StringVal)
					}

					stepContext.OutputVars[paramKey] = param
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

func (s *Matrix) build(params []v1beta1.Param) (map[string]map[string]string, error) {
	var keys []string
	mapData := make(map[string]v1beta1.ParamValue)

	for _, param := range params {
		keys = append(keys, param.Name)
		mapData[param.Name] = param.Value
	}

	result := make(map[string]map[string]string)

	s.generateCombinations(mapData, keys, 0, make(map[string]string), &result)

	return result, nil
}

func (s *Matrix) combineIncludes(matrixParams map[string]string, include []v1beta1.IncludeParam) {
	includeParams := make(map[string]string)

	for currentMatrixKey, currentMatrixValue := range matrixParams {
		for _, includeGroup := range include {
			combine := false
			for _, includeParam := range includeGroup.Params {
				if includeParam.Name == currentMatrixKey && includeParam.Value.StringVal == currentMatrixValue {
					combine = true
				}
			}

			if combine {
				for _, includeParam := range includeGroup.Params {
					includeParams[includeParam.Name] = includeParam.Value.StringVal
				}
			}
		}
	}

	maps.Copy(matrixParams, includeParams)
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
