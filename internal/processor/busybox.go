package processor

import (
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"

	"github.com/moby/buildkit/client/llb"
)

func WithBusybox() ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		if spec.Image == "" {
			return nil
		}

		return &Busybox{}
	}
}

type Busybox struct{}

func (s *Busybox) Bootstrap(_ Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		ctx.Build.State = bakeBusybox(ctx.Build.State)
		return next(ctx)
	}, nil
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
