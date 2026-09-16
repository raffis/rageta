package processor

import (
	"fmt"

	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"

	gwclient "github.com/moby/buildkit/frontend/gateway/client"
)

func WithInputFrom(gwClient gwclient.Client) ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		if spec.InputFrom == nil {
			return nil
		}

		return &InputFrom{
			gwClient: gwClient,
			inputs:   spec.InputFrom,
		}
	}
}

type InputFrom struct {
	gwClient gwclient.Client
	inputs   []v1beta1.InputFrom
}

func (s *InputFrom) Bootstrap(_ Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		inputs := make([]v1beta1.InputFrom, len(s.inputs))
		subst := []any{}

		for i := range inputs {
			inputs[i] = *s.inputs[i].DeepCopy()
			subst = append(subst, &inputs[i].Path)
		}

		if err := substitute.Substitute(ctx.ToV1Beta1(), subst...); err != nil {
			return ctx, err
		}

		var contextRef gwclient.Reference

		for _, input := range inputs {
			if input.From == nil {
				if contextRef == nil {
					contextDef, err := ctx.Build.ContextState.Marshal(ctx)
					if err != nil {
						return ctx, fmt.Errorf("marshal context failed: %w", err)
					}

					contextRes, err := s.gwClient.Solve(ctx, gwclient.SolveRequest{Definition: contextDef.ToPB()})
					if err != nil {
						return ctx, fmt.Errorf("solve context failed: %w", err)
					}

					contextRef = contextRes.Ref
				}

				vars, err := readVars(ctx, contextRef, input.Path)
				if err != nil {
					return ctx, fmt.Errorf("failed to read input vars from %q: %w", input.Path, err)
				}

				for k, v := range vars {
					ctx.InputVars.Inputs[k] = v1beta1.ParamValue{
						Type:      v1beta1.ParamTypeString,
						StringVal: v,
					}
				}
			} else {
				taskName := *input.From

				if instances, ok := ctx.TaskGroups[taskName]; ok {
					for _, stepCtx := range instances {
						vars, err := readVars(ctx, stepCtx.Build.Ref, input.Path)
						if err != nil {
							return ctx, fmt.Errorf("failed to read input vars from %q: %w", input.Path, err)
						}

						for k, v := range vars {
							ctx.InputVars.Inputs[k] = v1beta1.ParamValue{
								Type:      v1beta1.ParamTypeString,
								StringVal: v,
							}
						}

					}

					break
				}

				stepCtx, ok := ctx.Tasks[taskName]
				if !ok {
					return ctx, fmt.Errorf("source step %q dependency not found", taskName)
				}

				vars, err := readVars(ctx, stepCtx.Build.Ref, input.Path)
				if err != nil {
					return ctx, fmt.Errorf("failed to read input vars from %q: %w", input.Path, err)
				}

				for k, v := range vars {
					ctx.InputVars.Inputs[k] = v1beta1.ParamValue{
						Type:      v1beta1.ParamTypeString,
						StringVal: v,
					}
				}
			}
		}

		return next(ctx)
	}, nil
}
