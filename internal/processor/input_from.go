package processor

import (
	"fmt"

	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithInputFrom() ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		if spec.InputFrom == nil {
			return nil
		}

		return &InputFrom{
			items: spec.InputFrom,
		}
	}
}

type InputFrom struct {
	items []v1beta1.InputFrom
}

// Bootstrap reads dotenv-style files produced by this task's own build and
// merges their contents into ctx.InputVars.Inputs once the build has
// finished. It wraps WithInputVars so the values it adds survive that
// processor's post-run revert of ctx.InputVars.Inputs.
func (s *InputFrom) Bootstrap(_ Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		files := make([]string, len(s.items))
		subst := []any{}

		for i, item := range s.items {
			if item.File == nil {
				continue
			}

			files[i] = *item.File
			subst = append(subst, &files[i])
		}

		if err := substitute.Substitute(ctx.ToV1Beta1(), subst...); err != nil {
			return ctx, err
		}

		ctx, err := next(ctx)
		if err != nil {
			return ctx, err
		}

		for _, file := range files {
			if file == "" {
				continue
			}

			vars, err := readVars(ctx, ctx.Build.Ref, file)
			if err != nil {
				return ctx, fmt.Errorf("inputFrom %q: %w", file, err)
			}

			for k, v := range vars {
				ctx.InputVars.Inputs[k] = v1beta1.ParamValue{
					Type:      v1beta1.ParamTypeString,
					StringVal: v,
				}
			}
		}

		return ctx, nil
	}, nil
}
