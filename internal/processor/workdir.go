package processor

import (
	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"

	"github.com/moby/buildkit/client/llb"
)

func WithWorkdir() ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.WorkingDir == "" {
			return nil
		}
		return &Workdir{
			workingDir: spec.WorkingDir,
		}
	}
}

type Workdir struct {
	workingDir string
}

func (s *Workdir) Bootstrap(_ Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		workingDir := s.workingDir
		if err := substitute.Substitute(ctx.ToV1Beta1(), &workingDir); err != nil {
			return ctx, err
		}

		ctx.Build.State = ctx.Build.State.With(llb.Dir(workingDir))
		return next(ctx)
	}, nil
}
