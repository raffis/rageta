package processor

import (
	"sync"

	"github.com/raffis/rageta/internal/processor/shimbin"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"

	"github.com/moby/buildkit/client/llb"
)

func WithBusybox() ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		return &Busybox{}
	}
}

type Busybox struct{}

func (s *Busybox) Bootstrap(_ Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		ctx.Build.State = bakeBusybox(ctx.Build.State)
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

func bakeBusybox(state llb.State) llb.State {
	busybox := llb.Image("busybox:uclibc", llb.ResolveModePreferLocal)
	state = state.File(
		llb.Copy(busybox, "/bin/busybox", "/bin/", &llb.CopyInfo{
			CreateDestPath:                 true,
			AlwaysReplaceExistingDestPaths: false,
		}),
		llb.WithCustomNamef("copy busybox:%s → %s", "/*", "/"),
	)

	state = state.File(
		llb.Mkdir("/bin", 0755),
	)

	state = state.Run(
		llb.Shlex("/bin/busybox --install -s /bin"),
	).Root()

	return state
}
