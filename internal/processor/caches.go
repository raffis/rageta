package processor

import (
	"errors"
	"fmt"
	"strings"

	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"

	"github.com/moby/buildkit/client/llb"
)

func WithCaches() ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Caches == nil {
			return nil
		}
		return &Caches{
			caches: spec.Caches,
		}
	}
}

type Caches struct {
	caches []v1beta1.Cache
}

func (s *Caches) Bootstrap(_ Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		caches := make([]v1beta1.Cache, len(s.caches))
		subst := []any{}

		for i := range caches {
			caches[i] = *s.caches[i].DeepCopy()
			subst = append(subst, &caches[i].ID, &caches[i].Path)
		}
		if err := substitute.Substitute(ctx.ToV1Beta1(), subst...); err != nil {
			return ctx, err
		}

		for _, c := range caches {
			if c.ID == "" || c.Path == "" {
				return ctx, errors.New("cache mount requires id and path")
			}
			sharing := llb.CacheMountShared
			switch strings.ToLower(strings.TrimSpace(c.Sharing)) {
			case "", "shared":
			case "private":
				sharing = llb.CacheMountPrivate
			case "locked":
				sharing = llb.CacheMountLocked
			default:
				return ctx, fmt.Errorf("cache sharing %q: want shared, private, or locked", c.Sharing)
			}

			ctx.Build.RunOpts = append(ctx.Build.RunOpts, llb.AddMount(c.Path, llb.Scratch(), llb.AsPersistentCacheDir(c.ID, sharing)))
		}

		return next(ctx)
	}, nil
}
