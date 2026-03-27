package run

import (
	"os"

	"github.com/raffis/rageta/internal/mask"
	"github.com/spf13/pflag"
	"golang.org/x/term"
)

type SecretsOptions struct {
	Secrets []string
}

func (s *SecretsOptions) BindFlags(flags *pflag.FlagSet) {
	flags.StringSliceVarP(&s.Secrets, "secret", "s", s.Secrets, "Pass secrets to the pipeline. Secrets are handled as env variables but it is ensured they are masked in any sort of outputs.")
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

	rc.Output.Stdout = rc.Secrets.Store.Writer(rc.Output.Stdout)
	var isTerm = term.IsTerminal(int(os.Stdout.Fd()))

	if isTerm {
		rc.Output.Stderr = rc.Output.Stdout
	} else {
		rc.Output.Stderr = rc.Secrets.Store.Writer(rc.Output.Stderr)
	}

	return next(rc)
}
