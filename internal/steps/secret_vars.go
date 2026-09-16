package processor

import (
	"context"
	"fmt"
	"strings"

	"github.com/moby/buildkit/client/llb"
	"github.com/raffis/rageta/internal/secrets"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithSecretVars(osEnv map[string]string, store secrets.Interface) ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		secrets := secretMap(spec.Secrets, osEnv)
		for k, v := range secrets {
			store.AddSecret(context.Background(), k, []byte(v))
		}

		return &SecretVars{
			store: store,
		}
	}
}

type SecretVars struct {
	store secrets.Interface
}

type SecretVarsContext struct {
	Secrets map[string]string
}

func newSecretVarsContext() SecretVarsContext {
	return SecretVarsContext{
		Secrets: make(map[string]string),
	}
}

func (s *SecretVars) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		ctx.SecretVars.Secrets = newSecretVarsContext().Secrets

		for k, v := range s.store.Clone(ctx) {
			if strings.HasPrefix(k, ContextSecretPrefix) {
				continue
			}
			ctx.SecretVars.Secrets[k] = string(v)
		}

		for k, _ := range ctx.SecretVars.Secrets {
			ctx.Build.RunOpts = append(ctx.Build.RunOpts, llb.AddSecret(fmt.Sprintf("/run/secrets/%s", k), llb.SecretID(k)))
		}

		return next(ctx)
	}, nil
}

func secretMap(envs []v1beta1.SecretVar, osEnv map[string]string) map[string]string {
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
