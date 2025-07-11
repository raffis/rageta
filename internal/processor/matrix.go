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
	return func(ctx StepContext) (StepContext, error) {
		subst := []any{s.matrix}
		for _, group := range s.include {
			subst = append(subst, group.Params)
		}

		if err := substitute.Substitute(ctx.ToV1Beta1(),
			subst...,
		); err != nil {
			return ctx, fmt.Errorf("substitution failed: %w", err)
		}

		matrixes, err := s.build(s.matrix)
		if err != nil {
			return ctx, err
		}

		if len(matrixes) == 0 {
			return ctx, ErrEmptyMatrix
		}

		results := make(chan concurrentResult)
		var errs []error

		cancelCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		pool := s.pool
		if s.maxConcurrent > 0 {
			pool = s.pool.NewSubpool(s.maxConcurrent)
		}

		for matrixKey, matrix := range matrixes {
			copyCtx := ctx.DeepCopy()
			copyCtx.Context = cancelCtx
			copyCtx = s.extendMatrix(copyCtx, matrix, s.include)
			copyCtx.Matrix = matrix

			hasher := sha1.New()
			hasher.Write([]byte(matrixKey))
			b := hasher.Sum(nil)

			if copyCtx.NamePrefix == "" {
				copyCtx.NamePrefix = hex.EncodeToString(b)[:6]
			} else {
				copyCtx.NamePrefix = SuffixName(copyCtx.NamePrefix, hex.EncodeToString(b)[:6])
			}

			pool.Go(func() {
				t, err := next(copyCtx)
				results <- concurrentResult{t, err}
			})
		}

		var done int
	WAIT:
		for res := range results {
			done++

			for stepName, step := range res.ctx.Steps {
				ctx.Steps[SuffixName(stepName, res.ctx.NamePrefix)] = step
			}

			//Unify matrix outputs into an array output for the current step
			for paramKey, paramValue := range res.ctx.OutputVars {
				var param v1beta1.ParamValue

				if val, ok := ctx.OutputVars[paramKey]; !ok {
					param = v1beta1.ParamValue{
						Type: v1beta1.ParamTypeArray,
					}
				} else {
					param = val
				}

				if paramValue.Type == v1beta1.ParamTypeString && paramValue.StringVal != "" {
					param.ArrayVal = append(param.ArrayVal, paramValue.StringVal)
				}

				ctx.OutputVars[paramKey] = param
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
			return ctx, errors.Join(errs...)
		}

		return ctx, nil
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

func (s *Matrix) extendMatrix(ctx StepContext, matrixParams map[string]string, include []v1beta1.IncludeParam) StepContext {
	includeParams := make(map[string]string)

	for currentMatrixKey, currentMatrixValue := range matrixParams {
		tag := Tag{
			Key:   fmt.Sprintf("matrix/%s", currentMatrixKey),
			Value: currentMatrixValue,
		}

		for _, includeGroup := range include {
			combine := false
			for _, includeParam := range includeGroup.Params {
				if includeParam.Name == currentMatrixKey && includeParam.Value.StringVal == currentMatrixValue {
					combine = true
				}
			}

			if combine {
				tag.Color = includeGroup.Tag.Color

				if includeGroup.Tag.Value != "" {
					tag.Value = includeGroup.Tag.Value
				}

				for _, includeParam := range includeGroup.Params {
					includeParams[includeParam.Name] = includeParam.Value.StringVal
				}
			}
		}

		ctx = ctx.WithTag(tag)
	}

	maps.Copy(matrixParams, includeParams)
	return ctx
}

func (s *Matrix) generateCombinations(mapData map[string]v1beta1.ParamValue, keys []string, index int, currentCombination map[string]string, result *map[string]map[string]string) {
	// If we've added a value for each key, add the current combination to the result
	if index == len(keys) {
		// Create a copy of the current combination
		combinationCopy := make(map[string]string)
		maps.Copy(combinationCopy, currentCombination)

		// Create a unique key by concatenating values in currentCombination
		var combinationValues []string
		for _, key := range keys {
			val := currentCombination[key]
			// Handle different types (e.g., slices)
			if reflect.TypeOf(val).Kind() == reflect.Slice {
				// If it's a slice, join all its elements with a delimiter
				sliceVal := reflect.ValueOf(val)
				for i := range sliceVal.Len() {
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
