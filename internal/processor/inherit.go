package processor

import (
	"context"
	"fmt"
	"maps"

	"github.com/raffis/rageta/internal/provider"
	"github.com/raffis/rageta/internal/styles"
	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithInherit(builder PipelineBuilder, provider provider.Interface) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Inherit == nil {
			return nil
		}

		return &Inherit{
			stepName: spec.Name,
			step:     *spec.Inherit,
			provider: provider,
			builder:  builder,
		}
	}
}

type Inherit struct {
	builder  PipelineBuilder
	provider provider.Interface
	stepName string
	step     v1beta1.InheritStep
}

func (s *Inherit) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		inherit := s.step.DeepCopy()

		if err := substitute.Substitute(stepContext.ToV1Beta1(),
			inherit.Inputs,
		); err != nil {
			return stepContext, err
		}

		command, err := s.provider.Lookup(ctx, inherit.Pipeline)
		if err != nil {
			return stepContext, fmt.Errorf("failed to open pipeline: %w", err)
		}

		inheritCtx := stepContext.DeepCopy()
		inheritCtx.NamePrefix = suffixName(s.stepName, stepContext.NamePrefix)
		inheritCtx.Tags["inherit"] = Tag{
			Key:   "inherit",
			Value: inherit.Pipeline,
			Color: styles.RandHEXColor(0, 255),
		}

		cmd, err := s.builder.Build(command, inherit.Entrypoint, s.mapInputs(inherit.Inputs), inheritCtx)
		if err != nil {
			return stepContext, fmt.Errorf("failed to build pipeline: %w", err)
		}

		outputContext, outputs, err := cmd(ctx)

		if err != nil {
			return stepContext, fmt.Errorf("failed to execute pipeline: %w", err)
		}

		s.mergeContext(outputContext, stepContext)
		maps.Copy(stepContext.OutputVars, outputs)

		return next(ctx, stepContext)
	}, nil
}

func (s *Inherit) mapInputs(inputs []v1beta1.Param) map[string]v1beta1.ParamValue {
	m := make(map[string]v1beta1.ParamValue)
	for _, v := range inputs {
		m[v.Name] = v.Value
	}

	return m
}

func (s *Inherit) mergeContext(from, to StepContext) {
	maps.Copy(to.Envs, from.Envs)

	for k, v := range from.Steps {
		to.Steps[suffixName(k, s.stepName)] = v
	}

	for k, v := range from.Containers {
		to.Containers[suffixName(k, s.stepName)] = v
	}
}
