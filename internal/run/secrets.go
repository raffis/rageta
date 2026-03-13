package run

import (
	"github.com/raffis/rageta/internal/mask"
	"github.com/spf13/pflag"
)

type SecretsOptions struct {
	Secrets []string
}

func (s *SecretsOptions) BindFlags(flags *pflag.FlagSet) {
	flags.StringSliceVarP(&s.Secrets, "secret", "s", nil, "Pass secrets to the pipeline. Secrets are handled as env variables but it is ensured they are masked in any sort of outputs.")
}

func (s SecretsOptions) Build() Step {
	return &Secrets{opts: s}
}

type Secrets struct {
	opts SecretsOptions
}

type SecretsContext struct {
	Secrets map[string]string
	Store   *mask.SecretStore
}

func (s *Secrets) Run(rc *RunContext, next Next) error {
	rc.Secrets.Secrets = envMap(s.opts.Secrets)
	for _, secretValue := range rc.Secrets.Secrets {
		rc.Secrets.Store.AddSecrets([]byte(secretValue))
	}

	return next(rc)
}
