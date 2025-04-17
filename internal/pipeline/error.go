package pipeline

import (
	"errors"
	"fmt"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

var ErrInvalidInput = errors.New("invalid input")

func NewErrMissingInput(param v1beta1.InputParam) error {
	return fmt.Errorf("missing `%s (%s)`: %w", param.Name, param.Type, ErrInvalidInput)
}

func NewErrWrongInputType(param v1beta1.InputParam) error {
	return fmt.Errorf("wrong type `%s (%s)`: %w", param.Name, param.Type, ErrInvalidInput)
}

func NewErrUnknownInput(name string) error {
	return fmt.Errorf("unknown input `%s`: %w", name, ErrInvalidInput)
}
