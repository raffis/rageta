package processor

import (
	"context"
	"errors"
	"fmt"

	"github.com/joho/godotenv"
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

func (s *EnvFrom) Bootstrap(_ Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		ctx, err := next(ctx)
		if err != nil {
			return ctx, err
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
