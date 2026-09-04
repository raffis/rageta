package processor

import (
	"sync"

	"github.com/raffis/rageta/internal/processor/shimbin"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"

	"github.com/moby/buildkit/client/llb"
)

func WithShim() ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		if spec.Image == "" {
			return nil
		}

		return &Shim{}
	}
}

type Shim struct{}

func (s *Shim) Bootstrap(_ Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		ctx.Build.State = bakeShim(ctx.Build.State)
		return next(ctx)
	}, nil
}

const shimPath = "/rageta/shim"

var shimSource = sync.OnceValue(func() llb.State {
	return llb.Scratch().File(
		llb.Mkdir("/rageta", 0755),
	).File(
		llb.Mkfile(shimPath, 0755, shimbin.Binary),
		llb.WithCustomNamef("bake shim binary"),
	)
})

func bakeShim(state llb.State) llb.State {
	state = state.File(
		llb.Mkdir("/rageta", 0755),
	)

	return state.File(
		llb.Copy(shimSource(), shimPath, shimPath, &llb.CopyInfo{CreateDestPath: true}),
		llb.WithCustomNamef("copy shim binary"),
	)
}
