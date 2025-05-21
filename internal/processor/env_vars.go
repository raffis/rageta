package processor

import (
	"maps"
	"os"

	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithEnvVars(osEnv, defaultEnv map[string]string) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		return &EnvVars{
			env: envMap(spec.Env, osEnv, defaultEnv),
		}
	}
}

type EnvVars struct {
	env map[string]string
}

func (s *EnvVars) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		if err := substitute.Substitute(ctx.ToV1Beta1(),
			s.env,
		); err != nil {
			return ctx, err
		}

		maps.Copy(ctx.Envs, s.env)
		envTmp, err := os.CreateTemp(ctx.Dir, "env")
		if err != nil {
			return ctx, err
		}

		defer envTmp.Close()
		defer os.Remove(envTmp.Name())

		ctx.Env = envTmp.Name()
		ctx, nextErr := next(ctx)
		envTmp.Sync()

		envs, err := parseVars(envTmp)
		if err != nil {
			return ctx, err
		}

		maps.Copy(ctx.Envs, envs)
		return ctx, nextErr

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
