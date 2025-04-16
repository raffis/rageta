package processor

import (
	"context"
	"os"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithEnv(osEnv, defaultEnv map[string]string) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		return &Env{
			stepName:   spec.Name,
			defaultEnv: defaultEnv,
			stepEnv:    envMap(spec.Env, osEnv),
		}
	}
}

type Env struct {
	stepName   string
	stepEnv    map[string]string
	defaultEnv map[string]string
}

func (s *Env) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		if len(stepContext.Envs) == 0 && s.defaultEnv != nil {
			stepContext.Envs = s.defaultEnv
		}

		if err := Subst(stepContext.ToV1Beta1(),
			s.stepEnv,
		); err != nil {
			return stepContext, err
		}

		for k, v := range s.stepEnv {
			stepContext.Envs[k] = v
		}

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

		for k, v := range envs {
			stepContext.Envs[k] = v
		}

		return stepContext, nextErr

	}, nil
}

func envMap(envs []v1beta1.EnvVar, osEnv map[string]string) map[string]string {
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

	return env
}
