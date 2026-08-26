package processor

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/joho/godotenv"
	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"

	gwclient "github.com/moby/buildkit/frontend/gateway/client"
)

func WithEnvFrom() ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		if spec.EnvFrom == nil {
			return nil
		}

		return &EnvFrom{
			items: spec.EnvFrom,
		}
	}
}

type EnvFrom struct {
	items []v1beta1.EnvFrom
}

// Bootstrap reads dotenv-style files produced by this task's own build and
// merges their contents into ctx.EnvVars.Envs once the build has finished,
// so dependent/sibling tasks pick them up via TaskContext.Merge.
func (s *EnvFrom) Bootstrap(_ Pipeline, next Next) (Next, error) {
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
				return ctx, fmt.Errorf("envFrom %q: %w", file, err)
			}

			maps.Copy(ctx.EnvVars.Envs, vars)
		}

		return ctx, nil
	}, nil
}

func readVars(ctx context.Context, ref gwclient.Reference, srcPath string) (map[string]string, error) {
	stat, err := ref.StatFile(ctx, gwclient.StatRequest{Path: srcPath})
	if err != nil {
		return nil, err
	}

	if stat.IsDir() {
		return nil, errors.New("must be a file")
	}

	b, err := ref.ReadFile(ctx, gwclient.ReadRequest{Filename: srcPath})
	if err != nil {
		return nil, err
	}

	vars, err := godotenv.UnmarshalBytes(b)
	if err != nil {
		return vars, fmt.Errorf("vars parser failed: %w", err)
	}

	return vars, err
}
