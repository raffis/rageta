package processor

import (
	"context"
	"errors"
	"os"
	"path/filepath"

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
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		dataDir := filepath.Join(stepContext.dir, PrefixName(s.stepName, stepContext.NamePrefix), "_data")

		if _, err := os.Stat(dataDir); errors.Is(err, os.ErrNotExist) {
			err := os.MkdirAll(dataDir, 0700)
			if err != nil {
				return stepContext, err
			}
		}

		stepContext.dataDir = dataDir
		stepContext, err := next(ctx, stepContext)
		stepContext.Steps[s.stepName].DataDir = dataDir
		return stepContext, err
	}, nil
}
