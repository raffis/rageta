package processor

import (
	"context"
	"os"
	"strings"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithEnv(defaultEnv map[string]string) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		return &Env{
			stepName:   spec.Name,
			defaultEnv: defaultEnv,
			stepEnv:    envMap(spec.Env),
		}
	}
}

type Env struct {
	stepName   string
	stepEnv    map[string]string
	defaultEnv map[string]string
}

func (s *Env) Substitute() []interface{} {
	return []interface{}{s.stepEnv}
}

func (s *Env) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		if len(stepContext.Envs) == 0 && s.defaultEnv != nil {
			stepContext.Envs = s.defaultEnv
		}

		for k, v := range s.stepEnv {
			stepContext.Envs[k] = v
		}

		envTmp, err := os.CreateTemp(stepContext.TmpDir(), "env")
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

func envMap(envs []string) map[string]string {
	env := make(map[string]string)
	for _, e := range envs {
		s := strings.SplitN(e, "=", 2)
		env[s[0]] = s[1]
	}

	return env
}
