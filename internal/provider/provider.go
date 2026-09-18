package provider

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
)

type Interface interface {
	Resolve(ctx context.Context, ref string) (v1beta1.Pipeline, error)
}

type provider struct {
	decoder  runtime.Decoder
	handlers []Resolver
	validate func([]byte) error
}

type Resolver func(ctx context.Context, ref string) (io.Reader, error)

func New(decoder runtime.Decoder, handlers ...Resolver) *provider {
	return &provider{
		decoder:  decoder,
		handlers: handlers,
	}
}

// WithValidation configures a manifest validation function which is invoked
// against the raw manifest before it is decoded. A nil validate func disables
// validation.
func (s *provider) WithValidation(validate func([]byte) error) *provider {
	s.validate = validate
	return s
}

func (s *provider) Resolve(ctx context.Context, ref string) (v1beta1.Pipeline, error) {
	to := v1beta1.Pipeline{}
	var errs []error

	for _, handler := range s.handlers {
		if r, err := handler(ctx, ref); err == nil {
			manifest, err := io.ReadAll(r)
			if err != nil {
				return to, err
			}

			if s.validate != nil {
				if err := s.validate(manifest); err != nil {
					return to, fmt.Errorf("pipeline validation failed: %w", err)
				}
			}

			_, _, err = s.decoder.Decode(
				manifest,
				nil,
				&to)

			if err != nil {
				return to, err
			}

			return to, nil
		} else {
			errs = append(errs, err)
		}
	}

	return to, fmt.Errorf("could not lookup ref: %q: %w", ref, errors.Join(errs...))
}
