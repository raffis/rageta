package run

import (
	"os"

	"github.com/raffis/rageta/internal/secrets"
	"github.com/raffis/rageta/internal/setup/flagset"
	"golang.org/x/term"
)

type SecretsOptions struct {
	Secrets []string
}

func (s *SecretsOptions) BindFlags(flags flagset.Interface) {
	flags.StringSliceVarP(&s.Secrets, "secret", "s", s.Secrets, "Pass secrets to the pipeline. Secrets are handled as env variables but it is ensured they are masked in any sort of outputs.")
}

func (s SecretsOptions) Build() Step {
	return &Secrets{opts: s}
}

type Secrets struct {
	opts SecretsOptions
}

type SecretsContext struct {
	Store secrets.Interface
}

func (s *Secrets) Run(rc *RunContext, next Next) error {
	rc.Secrets.Store = secrets.InMemoryStore()
	for k, v := range envMap(s.opts.Secrets) {
		rc.Secrets.Store.AddSecret(rc, k, []byte(v))
	}

	rc.Output.Stdout = rc.Secrets.Store.Pipe(rc, rc.Output.Stdout, []byte("***"))
	var isTerm = term.IsTerminal(int(os.Stdout.Fd()))

	if isTerm {
		rc.Output.Stderr = rc.Output.Stdout
	} else {
		rc.Output.Stderr = rc.Secrets.Store.Pipe(rc, rc.Output.Stderr, []byte("***"))
	}

	return next(rc)
}
