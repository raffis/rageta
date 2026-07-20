package processor

import (
	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"

	"github.com/moby/buildkit/client/llb"
)

const (
	DefaultWorkingDir = "/rageta/work"
)

func WithWorkdir() ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		workDir := spec.WorkingDir
		if workDir == "" {
			workDir = DefaultWorkingDir
		}

		return &Workdir{
			workingDir: workDir,
		}
	}
}

type Workdir struct {
	workingDir string
}

type WorkdirContext struct {
	Path string
}

func (s *Workdir) Bootstrap(_ Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		ctx.Workdir.Path = s.workingDir
		if err := substitute.Substitute(ctx.ToV1Beta1(), &ctx.Workdir.Path); err != nil {
			return ctx, err
		}

		ctx.Build.State = ctx.Build.State.File(
			llb.Mkdir(ctx.Workdir.Path, 0755, llb.WithParents(true)),
			llb.WithCustomNamef("mkdir %s", ctx.Workdir.Path),
		).With(llb.Dir(ctx.Workdir.Path), llb.AddEnv("PWD", ctx.Workdir.Path))
		return next(ctx)
	}, nil
}
