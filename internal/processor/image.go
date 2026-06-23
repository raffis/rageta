package processor

import (
	"fmt"

	"github.com/moby/buildkit/client/llb"
	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithImage() ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		if spec.Image == "" {
			return nil
		}

		return &Image{
			image: spec.Image,
		}
	}
}

type Image struct {
	image string
}

func (s *Image) Bootstrap(_ Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		image := s.image
		if err := substitute.Substitute(ctx.ToV1Beta1(), &image); err != nil {
			return ctx, err
		}

		ctx.Build.State = llb.Image(image, llb.ResolveModePreferLocal)
		ctx, err := next(ctx)

		if err != nil {
			return ctx, &imageError{
				image:  image,
				parent: err,
			}
		}

		return ctx, err
	}, nil
}

type imageError struct {
	image  string
	parent error
}

func (e *imageError) Error() string {
	return fmt.Sprintf("image: %s", e.parent.Error())
}

func (e *imageError) Unwrap() error {
	return e.parent
}

func (e *imageError) Image() string {
	return e.image
}
