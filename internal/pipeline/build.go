package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	"github.com/raffis/rageta/internal/utils"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"

	"github.com/raffis/rageta/internal/processor"
)

type builder struct {
	logger      logr.Logger
	tmpDir      string
	stepBuilder StepBuilder
}

type builderOption func(*builder)
type StepBuilder func(spec v1beta1.Step, uniqueName string) []processor.Bootstraper

func WithLogger(logger logr.Logger) func(*builder) {
	return func(s *builder) {
		s.logger = logger
	}
}

func WithStepBuilder(stepBuilder StepBuilder) func(*builder) {
	return func(s *builder) {
		s.stepBuilder = stepBuilder
	}
}

func WithTmpDir(tmpDir string) func(*builder) {
	return func(s *builder) {
		s.tmpDir = tmpDir
	}
}

func NewBuilder(opts ...builderOption) *builder {
	e := &builder{
		logger: logr.Discard(),
		tmpDir: os.TempDir(),
	}

	for _, o := range opts {
		o(e)
	}

	return e
}

func (e *builder) mapInputs(params []v1beta1.InputParam, inputs map[string]v1beta1.ParamValue) (map[string]v1beta1.ParamValue, error) {
	result := make(map[string]v1beta1.ParamValue)
	for _, v := range params {
		v.SetDefaults()
		input, hasInput := inputs[v.Name]

		if v.Default == nil {
			result[v.Name] = v1beta1.ParamValue{
				Type: v.Type,
			}
		} else {
			result[v.Name] = *v.Default
		}

		if v.Default == nil && !hasInput {
			return result, NewErrMissingInput(v)
		}

		if hasInput {
			if input.Type != v.Type {
				return result, NewErrWrongInputType(v)
			}

			result[v.Name] = input
		}
	}

	return result, nil
}

func (e *builder) Build(pipeline v1beta1.Pipeline, entrypointName string, inputs map[string]v1beta1.ParamValue) (processor.Executable, error) {
	pipeline.SetDefaults()

	mappedInputs, err := e.mapInputs(pipeline.Inputs, inputs)
	if err != nil {
		return nil, err
	}

	e.logger.Info("build task from pipeline spec", "pipeline", pipeline, "inputs", mappedInputs)
	pipelineCtx, err := e.buildPipeline(pipeline)
	if err != nil {
		return nil, err
	}

	entrypoint, err := pipelineCtx.Entrypoint(entrypointName)

	if err != nil {
		return nil, err
	}

	contextDir := e.tmpDir

	if pipeline.PipelineSpec.Name != "" {
		contextDir = filepath.Join(contextDir, pipeline.PipelineSpec.Name)
	}

	if _, err := os.Stat(contextDir); errors.Is(err, os.ErrNotExist) {
		err := os.MkdirAll(contextDir, 0700)
		if err != nil {
			return nil, err
		}
	}

	return func(ctx context.Context) (processor.StepContext, error) {
		stepCtx := processor.NewContext(contextDir)
		stepCtx.Inputs = mappedInputs

		if err := recoverContext(stepCtx, contextDir); err != nil {
			return stepCtx, fmt.Errorf("failed to recover context: %w", err)
		}

		stepCtx.NamePrefix = pipeline.PipelineSpec.Name
		stepCtx, pipelineErr := entrypoint(ctx, stepCtx)

		if err := storeContext(stepCtx, contextDir); err != nil {
			if pipelineErr != nil {
				return stepCtx, fmt.Errorf("failed to store context: %w; pipeline error: %w", err, pipelineErr)
			}

			return stepCtx, fmt.Errorf("failed to store context: %w", err)
		}

		e.logger.V(1).Info("pipeline finished", "context", stepCtx.ToV1Beta1())

		return stepCtx, pipelineErr
	}, nil
}

func storeContext(stepCtx processor.StepContext, contextDir string) error {
	contextPath := filepath.Join(contextDir, "context.json")
	f, err := os.OpenFile(contextPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0700)
	if err != nil {
		return err
	}

	defer f.Close()

	b, err := json.Marshal(stepCtx.ToV1Beta1())
	if err != nil {
		return err
	}

	_, err = f.Write(b)
	return err
}

func recoverContext(stepCtx processor.StepContext, contextDir string) error {
	contextPath := filepath.Join(contextDir, "context.json")
	if _, err := os.Stat(contextPath); err == nil {
		f, err := os.Open(contextPath)
		if err != nil {
			return err
		}

		defer f.Close()

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

func (e *builder) buildPipeline(command v1beta1.Pipeline) (*pipeline, error) {
	p := &pipeline{
		name:       command.PipelineSpec.Name,
		id:         utils.RandString(5),
		entrypoint: command.PipelineSpec.Entrypoint,
	}

	for _, spec := range command.Steps {
		name := spec.Name
		origName := name
		processors := e.stepBuilder(spec, processor.PrefixName(spec.Name, command.PipelineSpec.Name))

		if err := p.withStep(origName, processors); err != nil {
			return p, err
		}
	}

	return p, nil
}
