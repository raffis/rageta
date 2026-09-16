package processor

import (
	"context"
	"encoding/json"
	"errors"
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

				if err := s.applyInputs(&ctx, contextRef, input.Path); err != nil {
					return ctx, err
				}
			} else {
				taskName := *input.From

				if instances, ok := ctx.TaskGroups[taskName]; ok {
					for _, stepCtx := range instances {
						if err := s.applyInputs(&ctx, stepCtx.Build.Ref, input.Path); err != nil {
							return ctx, err
						}
					}

					break
				}

				stepCtx, ok := ctx.Tasks[taskName]
				if !ok {
					return ctx, fmt.Errorf("source step %q dependency not found", taskName)
				}

				if err := s.applyInputs(&ctx, stepCtx.Build.Ref, input.Path); err != nil {
					return ctx, err
				}
			}
		}

		return next(ctx)
	}, nil
}

func (s *InputFrom) applyInputs(ctx *TaskContext, ref gwclient.Reference, srcPath string) error {
	b, err := readFile(ctx, ref, srcPath)
	if err != nil {
		return fmt.Errorf("failed to read input vars from %q: %w", srcPath, err)
	}

	vars := make(map[string]json.RawMessage)
	if err := json.Unmarshal(b, &vars); err != nil {
		return fmt.Errorf("failed to parse input vars from file: %w", err)
	}

	for k, v := range vars {
		var param v1beta1.ParamValue
		if err := param.UnmarshalJSON(v); err != nil {
			return fmt.Errorf("failed to parse input var %q: %w", k, err)
		}
		ctx.InputVars.Inputs[k] = param
	}

	return nil
}

func readFile(ctx context.Context, ref gwclient.Reference, srcPath string) ([]byte, error) {
	stat, err := ref.StatFile(ctx, gwclient.StatRequest{Path: srcPath})
	if err != nil {
		return nil, err
	}

	if stat.IsDir() {
		return nil, errors.New("must be a file")
	}

	return ref.ReadFile(ctx, gwclient.ReadRequest{Filename: srcPath})
}
