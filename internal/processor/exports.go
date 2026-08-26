package processor

import (
	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"

	"github.com/moby/buildkit/client/llb"
	gwclient "github.com/moby/buildkit/frontend/gateway/client"
)

func WithExports(gwClient gwclient.Client) ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		if spec.Exports == nil {
			return nil
		}

		return &Exports{
			gwClient: gwClient,
			exports:  spec.Exports,
		}
	}
}

type Exports struct {
	gwClient gwclient.Client
	exports  []v1beta1.Export
}

func (s *Exports) Bootstrap(_ Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		paths := make([]string, len(s.exports))
		subst := []any{}

		for i, item := range s.exports {
			if item.Path == nil {
				continue
			}

			paths[i] = *item.Path
			subst = append(subst, &paths[i])
		}

		if err := substitute.Substitute(ctx.ToV1Beta1(), subst...); err != nil {
			return ctx, err
		}

		ctx, err := next(ctx)
		if err != nil {
			return ctx, err
		}

		if ctx.Build.DebugState == nil {
			full := ctx.Build.State
			ctx.Build.DebugState = &full
		}

		export := llb.Scratch()
		for _, path := range paths {
			if path == "" {
				continue
			}

			export = export.File(llb.Copy(ctx.Build.State, path, path, &llb.CopyInfo{
				CreateDestPath:      true,
				CopyDirContentsOnly: true,
				AllowWildcard:       true,
			}))
		}

		ctx.Build.State = export
		return ctx, nil
	}, nil
}
