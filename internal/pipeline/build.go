package pipeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	"github.com/raffis/rageta/internal/runtime"
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
type StepBuilder func(spec v1beta1.Step) []processor.Bootstraper

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
	for _, expectedInput := range params {
		expectedInput.SetDefaults()
		userInput, hasInput := inputs[expectedInput.Name]

		if expectedInput.Default == nil {
			result[expectedInput.Name] = v1beta1.ParamValue{
				Type: expectedInput.Type,
			}
		} else {
			result[expectedInput.Name] = *expectedInput.Default
		}

		if expectedInput.Default == nil && !hasInput {
			return result, NewErrMissingInput(expectedInput)
		}

		if hasInput {
			if userInput.Type != expectedInput.Type {
				return result, NewErrWrongInputType(expectedInput, userInput)
			}

			result[expectedInput.Name] = userInput
		}
	}

	for name := range inputs {
		if _, ok := result[name]; !ok {
			return result, NewErrUnknownInput(name)
		}
	}

	return result, nil
}

func (e *builder) Build(pipeline v1beta1.Pipeline, entrypointName string, inputs map[string]v1beta1.ParamValue, stepCtx processor.StepContext) (processor.Executable, error) {
	pipeline.SetDefaults()

	mappedInputs, err := e.mapInputs(pipeline.Inputs, inputs)
	if err != nil {
		return nil, err
	}

	e.logger.V(1).Info("build task from pipeline spec", "pipeline", pipeline, "inputs", mappedInputs)
	pipelineCtx, err := e.buildPipeline(pipeline)
	if err != nil {
		return nil, err
	}

	entrypoint, err := pipelineCtx.Entrypoint(entrypointName)

	if err != nil {
		return nil, err
	}

	contextDir := e.tmpDir

	if pipeline.Name != "" {
		contextDir = filepath.Join(contextDir, pipeline.Name)
	}

	return func() (processor.StepContext, map[string]v1beta1.ParamValue, error) {
		stepCtx.Dir = contextDir
		stepCtx.DataDir = filepath.Join(contextDir, "_data")
		stepCtx.Containers = make(map[string]runtime.ContainerStatus)
		stepCtx.Steps = make(map[string]*processor.StepContext)
		stepCtx.Inputs = mappedInputs
		outputs := make(map[string]v1beta1.ParamValue)

		if _, err := os.Stat(stepCtx.DataDir); errors.Is(err, os.ErrNotExist) {
			err := os.MkdirAll(stepCtx.DataDir, 0700)
			if err != nil {
				return stepCtx, outputs, fmt.Errorf("failed to create context dir: %w", err)
			}
		}

		if err := recoverContext(stepCtx, contextDir); err != nil {
			return stepCtx, outputs, fmt.Errorf("failed to recover context: %w", err)
		}

		stepCtx, pipelineErr := entrypoint(stepCtx)

		for _, pipelineOutput := range pipeline.Outputs {
			if _, ok := stepCtx.Steps[pipelineOutput.Step.Name]; !ok {
				continue
			}

			from := pipelineOutput.Name
			if pipelineOutput.From != "" {
				from = pipelineOutput.From
			}

			if output, ok := stepCtx.OutputVars[from]; ok {
				outputs[pipelineOutput.Name] = output
			}
		}

		if err := storeContext(stepCtx, contextDir); err != nil {
			if pipelineErr != nil {
				return stepCtx, outputs, fmt.Errorf("failed to store context: %w; pipeline error: %w", err, pipelineErr)
			}

			return stepCtx, outputs, fmt.Errorf("failed to store context: %w", err)
		}

		e.logger.V(1).Info("pipeline finished", "context", stepCtx.ToV1Beta1())

		return stepCtx, outputs, pipelineErr
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
		name:       command.Name,
		id:         utils.RandString(5),
		entrypoint: command.PipelineSpec.Entrypoint,
	}

	for _, spec := range command.Steps {
		name := spec.Name
		origName := name
		processors := e.stepBuilder(spec)

		if err := p.withStep(origName, processors); err != nil {
			return p, err
		}
	}

	return p, nil
}
