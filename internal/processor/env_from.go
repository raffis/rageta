package processor

import (
	"fmt"
	"maps"

	"github.com/joho/godotenv"
	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"

	gwclient "github.com/moby/buildkit/frontend/gateway/client"
)

func WithEnvFrom(gwClient gwclient.Client) ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		if spec.EnvFrom == nil {
			return nil
		}

		return &EnvFrom{
			gwClient: gwClient,
			envs:     spec.EnvFrom,
		}
	}
}

type EnvFrom struct {
	gwClient gwclient.Client
	envs     []v1beta1.EnvFrom
}

func (s *EnvFrom) Bootstrap(_ Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		envs := make([]v1beta1.EnvFrom, len(s.envs))
		subst := []any{}

		for i := range envs {
			envs[i] = *s.envs[i].DeepCopy()
			subst = append(subst, &envs[i].Src)
		}

		if err := substitute.Substitute(ctx.ToV1Beta1(), subst...); err != nil {
			return ctx, err
		}

		var contextRef gwclient.Reference

		for _, env := range envs {
			if env.From == nil {
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

				if err := s.applyEnvs(&ctx, contextRef, env.Src); err != nil {
					return ctx, err
				}
			} else {
				taskName := *env.From

				if instances, ok := ctx.TaskGroups[taskName]; ok {
					for _, stepCtx := range instances {
						if err := s.applyEnvs(stepCtx, stepCtx.Build.Ref, env.Src); err != nil {
							return ctx, err
						}
					}

					break
				}

				stepCtx, ok := ctx.Tasks[taskName]
				if !ok {
					return ctx, fmt.Errorf("source step %q dependency not found", taskName)
				}

				if err := s.applyEnvs(stepCtx, stepCtx.Build.Ref, env.Src); err != nil {
					return ctx, err
				}
			}
		}

		return next(ctx)
	}, nil
}

func (s *EnvFrom) applyEnvs(ctx *TaskContext, ref gwclient.Reference, srcPath string) error {
	b, err := readFile(ctx, ref, srcPath)
	if err != nil {
		return fmt.Errorf("failed to read env vars from %q: %w", srcPath, err)
	}

	vars, err := godotenv.UnmarshalBytes(b)
	if err != nil {
		return fmt.Errorf("invalid .env file from %q: %w", srcPath, err)
	}

	maps.Copy(ctx.EnvVars.Envs, vars)
	return nil
}
