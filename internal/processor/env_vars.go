package processor

import (
	"maps"
	"os"
	"path"

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

type EnvVarsContext struct {
	Envs       map[string]string
	OutputPath string
}

func newEnvVarsContext() EnvVarsContext {
	return EnvVarsContext{
		Envs: make(map[string]string),
	}
}

func (s *EnvVars) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		originEnvs := make(map[string]string, len(ctx.EnvVars.Envs))
		maps.Copy(originEnvs, ctx.EnvVars.Envs)
		maps.Copy(ctx.EnvVars.Envs, s.env)
		if err := substitute.Substitute(ctx.ToV1Beta1(),
			ctx.EnvVars.Envs,
		); err != nil {
			return ctx, err
		}

		envTmp, err := os.CreateTemp(path.Join(ctx.ContextDir, ctx.UniqueID()), "env")
		if err != nil {
			return ctx, err
		}

		var nextErr error
		defer func() {
			_ = envTmp.Close()
			_ = os.Remove(envTmp.Name())
		}()

		ctx.EnvVars.OutputPath = envTmp.Name()
		ctx, nextErr = next(ctx)
		if syncErr := envTmp.Sync(); syncErr != nil {
			nextErr = syncErr
		}

		envs, err := parseVars(envTmp)
		if err != nil {
			return ctx, err
		}

		maps.Copy(originEnvs, envs)
		ctx.EnvVars.Envs = originEnvs
		ctx.EnvVars.OutputPath = ""

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
