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
}

type Resolver func(ctx context.Context, ref string) (io.Reader, error)

func New(decoder runtime.Decoder, handlers ...Resolver) *provider {
	return &provider{
		decoder:  decoder,
		handlers: handlers,
	}
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
