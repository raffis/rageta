package pipeline

import (
	"github.com/go-logr/logr"
	"github.com/raffis/rageta/internal/utils"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"

	"github.com/raffis/rageta/internal/processor"
)

type builder struct {
	logger      logr.Logger
	stepBuilder TaskBuilder
}

type builderOption func(*builder)
type TaskBuilder func(spec v1beta1.Task) []processor.Bootstraper

func WithLogger(logger logr.Logger) func(*builder) {
	return func(s *builder) {
		s.logger = logger
	}
}

func WithTaskBuilder(stepBuilder TaskBuilder) func(*builder) {
	return func(s *builder) {
		s.stepBuilder = stepBuilder
	}
}

func NewBuilder(opts ...builderOption) *builder {
	e := &builder{
		logger: logr.Discard(),
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

func (e *builder) Build(pipeline v1beta1.Pipeline, entrypointName string, inputs map[string]v1beta1.ParamValue, stepCtx processor.TaskContext) (processor.Executable, error) {
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

	return func() (processor.TaskContext, map[string]v1beta1.ParamValue, error) {
		stepCtx.Tasks = make(map[string]*processor.TaskContext)
		stepCtx.InputVars.Inputs = mappedInputs
		inheritedState := stepCtx.Build.State
		stepCtx.Build.ContextState = &inheritedState
		outputs := make(map[string]v1beta1.ParamValue)

		stepCtx, pipelineErr := entrypoint(stepCtx)
		e.logger.V(1).Info("pipeline finished", "context", stepCtx.ToV1Beta1())
		return stepCtx, outputs, pipelineErr
	}, nil
}

func (e *builder) buildPipeline(command v1beta1.Pipeline) (*pipeline, error) {
	p := &pipeline{
		name:       command.Name,
		id:         utils.RandString(5),
		entrypoint: command.Entrypoint,
	}

	steps, err := resolveTemplates(command.Tasks, command.Templates)
	if err != nil {
		return nil, err
	}

	for _, spec := range steps {
		processors := e.stepBuilder(spec)

		var refs []string
		for _, ref := range spec.DependsOn {
			if ref.Name != nil {
				refs = append(refs, *ref.Name)
			} else if ref.MatchLabels != nil {
				for _, s := range steps {
					if matchLabels(specLabels(s), ref.MatchLabels) {
						refs = append(refs, s.Name)
					}
				}
			}
		}

		labels := specLabels(spec)
		if err := p.withTask(spec.Name, refs, labels, processors); err != nil {
			return p, err
		}
	}

	return p, nil
}
