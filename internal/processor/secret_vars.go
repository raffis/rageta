package processor

import (
	"maps"
	"os"
	"path"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

type secretWriter interface {
	AddSecrets(secrets ...[]byte)
}

func WithSecretVars(osEnv, defaultSecret map[string]string, secretWriter secretWriter) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		secrets := secretMap(spec.Secrets, osEnv, defaultSecret)
		for _, v := range secrets {
			secretWriter.AddSecrets([]byte(v))
		}

		return &SecretVars{
			secret:       secrets,
			secretWriter: secretWriter,
		}
	}
}

type SecretVars struct {
	secret       map[string]string
	secretWriter secretWriter
}

type SecretVarsContext struct {
	Secrets    map[string]string
	OutputPath string
}

func newSecretVarsContext() SecretVarsContext {
	return SecretVarsContext{
		Secrets: make(map[string]string),
	}
}

func (s *SecretVars) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		originSecrets := make(map[string]string, len(ctx.SecretVars.Secrets))
		maps.Copy(originSecrets, ctx.SecretVars.Secrets)

		maps.Copy(ctx.SecretVars.Secrets, s.secret)
		secretTmp, err := os.CreateTemp(path.Join(ctx.ContextDir, ctx.UniqueID()), "secret")
		if err != nil {
			return ctx, err
		}

		var nextErr error
		defer func() {
			_ = secretTmp.Close()
			_ = os.Remove(secretTmp.Name())
		}()

		ctx.SecretVars.OutputPath = secretTmp.Name()
		ctx, nextErr = next(ctx)
		if syncErr := secretTmp.Sync(); syncErr != nil {
			nextErr = syncErr
		}

		secrets, err := parseVars(secretTmp)
		if err != nil {
			return ctx, err
		}

		for _, v := range secrets {
			s.secretWriter.AddSecrets([]byte(v))
		}

		maps.Copy(originSecrets, secrets)
		ctx.SecretVars.Secrets = originSecrets
		ctx.SecretVars.OutputPath = ""

		return ctx, nextErr

	}, nil
}

func secretMap(envs []v1beta1.SecretVar, osEnv, defaultEnv map[string]string) map[string]string {
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
