package processor

import (
	"errors"
	"fmt"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithAllowFailure() ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if !spec.AllowFailure {
			return nil
		}

		return &AllowFailure{}
	}
}

type AllowFailure struct {
}

var ErrAllowFailure = errors.New("ignore error returned from step")

func (s *AllowFailure) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		ctx, err := next(ctx)

		if err != nil {
			err = fmt.Errorf("%w: %w", ErrAllowFailure, err)
		}

		return ctx, err
	}, nil
}
