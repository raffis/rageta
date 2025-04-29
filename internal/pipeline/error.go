package pipeline

import (
	"errors"
	"fmt"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

var ErrInvalidInput = errors.New("invalid input")

func NewErrMissingInput(expectedInput v1beta1.InputParam) error {
	return fmt.Errorf("missing input `%s (%s)`: %w", expectedInput.Name, expectedInput.Type, ErrInvalidInput)
}

func NewErrWrongInputType(expectedInput v1beta1.InputParam, userInput v1beta1.ParamValue) error {
	return fmt.Errorf("wrong input type `%s` for `%s` given, expected `%s`: %w", userInput.Type, expectedInput.Name, expectedInput.Type, ErrInvalidInput)
}

func NewErrUnknownInput(name string) error {
	return fmt.Errorf("unknown input `%s`: %w", name, ErrInvalidInput)
}
