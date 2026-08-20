package processor

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/distribution/reference"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/client/llb/sourceresolver"
	gwclient "github.com/moby/buildkit/frontend/gateway/client"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithImage(gwClient gwclient.Client) ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		if spec.Image == "" {
			return nil
		}

		return &Image{
			image:    spec.Image,
			gwClient: gwClient,
		}
	}
}

type Image struct {
	image    string
	gwClient gwclient.Client
}

func (s *Image) Bootstrap(_ Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		image := s.image
		if err := substitute.Substitute(ctx.ToV1Beta1(), &image); err != nil {
			return ctx, err
		}

		ctx.Build.State = llb.Merge([]llb.State{ctx.Build.State, llb.Image(image, llb.ResolveModePreferLocal)})

		normalizedRef := image
		if r, refErr := reference.ParseNormalizedNamed(image); refErr == nil {
			normalizedRef = reference.TagNameOnly(r).String()
		}

		_, _, config, err := s.gwClient.ResolveImageConfig(ctx, normalizedRef, sourceresolver.Opt{
			ImageOpt: &sourceresolver.ResolveImageOpt{
				ResolveMode: llb.ResolveModePreferLocal.String(),
			},
		})
		if err != nil {
			return ctx, &imageError{image: image, parent: err}
		}

		var imgConfig ocispecs.Image
		if err := json.Unmarshal(config, &imgConfig); err != nil {
			return ctx, &imageError{image: image, parent: err}
		}

		for _, kv := range imgConfig.Config.Env {
			if k, v, ok := strings.Cut(kv, "="); ok {
				ctx.EnvVars.Envs[k] = v
			}
		}

		ctx, err = next(ctx)

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
