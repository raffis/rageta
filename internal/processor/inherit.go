package processor

import (
	"context"
	"fmt"

	"github.com/raffis/rageta/internal/storage"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithInherit(builder PipelineBuilder, store storage.Interface) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Inherit == nil {
			return nil
		}

		return &Inherit{
			stepName: spec.Name,
			step:     *spec.Inherit,
			store:    store,
			builder:  builder,
		}
	}
}

type Inherit struct {
	builder  PipelineBuilder
	store    storage.Interface
	stepName string
	step     v1beta1.InheritStep
}

func (s *Inherit) Substitute() []*Substitute {
	var vals []*Substitute
	vals = append(vals, &Substitute{
		v: s.step.Pipeline,
		f: func(v interface{}) {
			s.step.Pipeline = v.(string)
		},
	})
	return vals
}

func (s *Inherit) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		command, err := s.store.Lookup(ctx, s.step.Pipeline)
		if err != nil {
			return stepContext, fmt.Errorf("failed to open pipeline: %w", err)
		}

		command.PipelineSpec.Name = PrefixName(s.stepName, stepContext.NamePrefix)

		cmd, err := s.builder.Build(command, s.step.Entrypoint, s.mapInputs(s.step.Inputs))
		if err != nil {
			return stepContext, fmt.Errorf("failed to build pipeline: %w", err)
		}

		outputContext, err := cmd(ctx)

		if err != nil {
			return stepContext, fmt.Errorf("failed to execute pipeline: %w", err)
		}

		s.mergeContext(outputContext, stepContext)
		return next(ctx, stepContext)
	}, nil
}

func (s *Inherit) mapInputs(inputs []v1beta1.InheritInput) map[string]string {
	m := make(map[string]string)
	for _, v := range inputs {
		m[v.Name] = v.Value
	}

	return m
}

func (s *Inherit) mergeContext(from, to StepContext) {
	for k, v := range from.Envs {
		to.Envs[k] = v
	}

	for k, v := range from.Steps {
		to.Steps[PrefixName(k, s.stepName)] = v
	}

	for k, v := range from.Containers {
		to.Containers[PrefixName(k, s.stepName)] = v
	}
}
