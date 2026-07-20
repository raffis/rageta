package processor

import (
	"fmt"

	"github.com/raffis/rageta/internal/provider"
	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithInherit(builder PipelineBuilder, provider provider.Interface) ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
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
	step     v1beta1.InheritTask
}

func (s *Inherit) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		inherit := s.step.DeepCopy()

		if err := substitute.Substitute(ctx.ToV1Beta1(),
			inherit.Inputs,
		); err != nil {
			return ctx, err
		}

		pipe, err := s.provider.Resolve(ctx, inherit.Pipeline)
		if err != nil {
			return ctx, fmt.Errorf("failed to resolve pipeline: %w", err)
		}

		inheritCtx := ctx.DeepCopy().WithNamespace(s.stepName)
		inheritCtx.Labels.Add(Label{
			Key:   "pipeline",
			Value: pipe.Name,
		})

		cmd, err := s.builder.Build(pipe, inherit.Entrypoint, s.mapInputs(inherit.Inputs), inheritCtx)
		if err != nil {
			return ctx, fmt.Errorf("failed to build pipeline: %w", err)
		}

		outputCtx, _, err := cmd()

		if err != nil {
			return ctx, fmt.Errorf("failed to execute pipeline: %w", err)
		}

		ctx.Build.State = outputCtx.Build.State
		ctx.Build.Ref = outputCtx.Build.Ref
		ctx.Build.Cached = outputCtx.Build.Cached

		return next(ctx)
	}, nil
}

func (s *Inherit) mapInputs(inputs []v1beta1.Param) map[string]v1beta1.ParamValue {
	m := make(map[string]v1beta1.ParamValue)
	for _, v := range inputs {
		m[v.Name] = v.Value
	}

	return m
}
