package processor

import (
	"context"
	"os"

	"maps"

	"github.com/raffis/rageta/internal/mask"
	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithEnvVars(osEnv, defaultEnv map[string]string, secretWriter mask.SecretStore) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		return &EnvVars{
			stepName:     spec.Name,
			env:          envMap(spec.Env, osEnv, defaultEnv),
			secretWriter: secretWriter,
		}
	}
}

type EnvVars struct {
	stepName     string
	env          map[string]string
	secretWriter mask.SecretStore
}

func (s *EnvVars) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		if err := substitute.Substitute(stepContext.ToV1Beta1(),
			s.env,
		); err != nil {
			return stepContext, err
		}

		maps.Copy(stepContext.Envs, s.env)
		envTmp, err := os.CreateTemp(stepContext.Dir, "env")
		if err != nil {
			return stepContext, err
		}

		defer envTmp.Close()
		defer os.Remove(envTmp.Name())

		stepContext.Env = envTmp.Name()
		stepContext, nextErr := next(ctx, stepContext)
		envTmp.Sync()

		envs, err := parseVars(envTmp)
		if err != nil {
			return stepContext, err
		}

		maps.Copy(stepContext.Envs, envs)
		return stepContext, nextErr

	}, nil
}

func envMap(envs []v1beta1.EnvVar, osEnv, defaultEnv map[string]string) map[string]string {
	env := make(map[string]string)
	for _, e := range envs {
		if e.Value == nil {
			if v, ok := osEnv[e.Name]; ok {
				env[e.Name] = v
			}

			continue
		}

		env[e.Name] = *e.Value
	}

	maps.Copy(env, defaultEnv)
	return env
}
