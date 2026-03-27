package run

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"github.com/spf13/pflag"
)

type StepContextOptions struct {
	RecoverFrom string
}

func (s StepContextOptions) Build() Step {
	return &StepContext{
		opts: s,
	}
}

func (s *StepContextOptions) BindFlags(flags *pflag.FlagSet) {
	flags.StringVarP(&s.RecoverFrom, "recover", "", s.RecoverFrom, "Recover from previous execution. Path to context directory.")
}

type StepContext struct {
	opts StepContextOptions
}

type StepContextContext struct {
}

func (s *StepContext) Run(rc *RunContext, next Next) error {
	stepCtx := processor.NewContext()
	rc.Execution.StepContext = stepCtx

	if err := s.recoverContext(&stepCtx, s.opts.RecoverFrom); err != nil {
		return err
	}

	rc.Execution.StepContext = stepCtx
	err := next(rc)

	if storeErr := s.storeContext(rc.Execution.StepContext, rc.ContextDir.Path); storeErr != nil {
		return errors.Join(err, storeErr)
	}

	return err
}

func (s *StepContext) storeContext(stepCtx processor.StepContext, contextDir string) error {
	contextPath := filepath.Join(contextDir, "context.json")
	f, err := os.OpenFile(contextPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0700)
	if err != nil {
		return err
	}

	defer func() {
		_ = f.Close()
	}()

	b, err := json.Marshal(stepCtx.ToV1Beta1())
	if err != nil {
		return err
	}

	_, err = f.Write(b)
	return err
}

func (s *StepContext) recoverContext(stepCtx *processor.StepContext, contextDir string) error {
	contextPath := filepath.Join(contextDir, "context.json")
	if _, err := os.Stat(contextPath); err == nil {
		f, err := os.Open(contextPath)
		if err != nil {
			return err
		}

		defer func() {
			_ = f.Close()
		}()

		vars := &v1beta1.Context{}

		b, err := io.ReadAll(f)
		if err != nil {
			return err
		}

		err = json.Unmarshal(b, vars)
		if err != nil {
			return err
		}

		stepCtx.FromV1Beta1(vars)
	}

	return nil
}
