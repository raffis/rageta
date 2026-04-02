package processor

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"maps"

	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithMatrix() ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Matrix == nil || len(spec.Matrix.Params) == 0 {
			return nil
		}

		return &Matrix{
			matrix:   spec.Matrix.Params,
			include:  spec.Matrix.Include,
			failFast: spec.Matrix.FailFast,
			stepName: spec.Name,
			pool:     make(chan struct{}, spec.Matrix.MaxConcurrent),
		}
	}
}

type Matrix struct {
	matrix   []v1beta1.Param
	include  []v1beta1.IncludeParam
	failFast bool
	stepName string
	pool     chan struct{}
}

type MatrixContext struct {
	Params map[string]string
}

func newMatrixContext() MatrixContext {
	return MatrixContext{
		Params: make(map[string]string),
	}
}

var ErrEmptyMatrix = &pipelineError{
	message:      "matrix is empty",
	result:       "empty-matrix",
	abortOnError: false,
}

type isMatrixContext struct{}

func (s *Matrix) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		if ctx.Value(isMatrixContext{}) == s {
			return next(ctx)
		}

		matrixParams := slices.Clone(s.matrix)
		matrixWrap := []any{
			matrixParams,
		}
		if err := substitute.Substitute(ctx.ToV1Beta1(),
			matrixWrap...,
		); err != nil {
			return ctx, fmt.Errorf("substitution failed for matrix parameters: %w", err)
		}

		additionalParams := slices.Clone(s.include)
		additionalParamsWrap := []any{}

		for k, v := range additionalParams {
			additionalParams[k].Params = slices.Clone(v.Params)
			additionalParamsWrap = append(additionalParamsWrap, additionalParams[k].Params)
		}

		if err := substitute.Substitute(ctx.ToV1Beta1(),
			additionalParamsWrap...,
		); err != nil {
			return ctx, fmt.Errorf("substitution failed for include matrix parameters: %w", err)
		}

		//If a matrix combination needs to be processed the step needs to start from beginning in order to through all step
		//processors
		next, err := pipeline.Entrypoint(s.stepName)
		if err != nil {
			return ctx, err
		}

		ctx.Context = context.WithValue(ctx, isMatrixContext{}, s)

		matrixes, err := s.build(matrixParams)
		if err != nil {
			return ctx, err
		}

		if len(matrixes) == 0 {
			return ctx, ErrEmptyMatrix
		}

		results := make(chan result)
		var errs []error

		cancelCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		for matrixKey, matrix := range matrixes {
			hasher := sha1.New()
			hasher.Write([]byte(matrixKey))
			b := hasher.Sum(nil)

			copyCtx := ctx.DeepCopy().WithNamespace(fmt.Sprintf("%x", b)[:6])
			copyCtx.Context = cancelCtx
			copyCtx = s.extendMatrix(copyCtx, matrix, additionalParams)
			copyCtx.Matrix.Params = matrix

			go func() {
				if cap(s.pool) > 0 {
					s.pool <- struct{}{}
					defer func() {
						<-s.pool
					}()
				}

				t, err := next(copyCtx)
				results <- result{t, err}
			}()
		}

		var done int
	WAIT:
		for res := range results {
			done++
			maps.Copy(ctx.Steps, res.ctx.Steps)

			//Unify matrix outputs into an array output for the current step
			for paramKey, paramValue := range res.ctx.OutputVars.OutputVars {
				var param v1beta1.ParamValue

				if val, ok := ctx.OutputVars.OutputVars[paramKey]; !ok {
					param = v1beta1.ParamValue{
						Type: v1beta1.ParamTypeArray,
					}
				} else {
					param = val
				}

				if paramValue.Type == v1beta1.ParamTypeString && paramValue.StringVal != "" {
					param.ArrayVal = append(param.ArrayVal, paramValue.StringVal)
				}

				ctx.OutputVars.OutputVars[paramKey] = param
			}

			switch {
			case cancelCtx.Err() == context.Canceled && len(errs) > 0:
			case res.err != nil && AbortOnError(res.err):
				errs = append(errs, res.err)

				if s.failFast {
					cancel()
				}
			default:
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

		ctx.Tags.Add(tag)
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
			if reflect.TypeOf(val).Kind() == reflect.Slice {
				sliceVal := reflect.ValueOf(val)
				for i := range sliceVal.Len() {
					combinationValues = append(combinationValues, fmt.Sprintf("%v", sliceVal.Index(i).Interface()))
				}
			} else {
				combinationValues = append(combinationValues, fmt.Sprintf("%v", val))
			}
		}

		uniqueKey := strings.Join(combinationValues, "-")
		(*result)[uniqueKey] = combinationCopy
		return
	}

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
