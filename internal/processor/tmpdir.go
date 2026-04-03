package processor

import (
	"errors"
	"os"
	"path"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithTmpDir() ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		return &TmpDir{
			stepName: spec.Name,
		}
	}
}

type TmpDir struct {
	stepName string
}

func (s *TmpDir) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		dataDir := path.Join(ctx.ContextDir, ctx.UniqueID(), "data")

		if _, err := os.Stat(dataDir); errors.Is(err, os.ErrNotExist) {
			err := os.MkdirAll(dataDir, 0700)
			if err != nil {
				return ctx, err
			}
		}

		ctx, err := next(ctx)
		return ctx, err
	}, nil
}
