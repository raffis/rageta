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
	flags.StringSliceVarP(&s.Secrets, "secret", "", s.Secrets, "Pass secrets to the pipeline. Secrets are loaded from a secret backend and it is ensured secrets on any streams are always masked.")
}

func (s SecretsOptions) Build() Task {
	return &Secrets{opts: s}
}

type Secrets struct {
	opts SecretsOptions
}

type SecretsContext struct {
	Store secrets.Interface
}

func (s *Secrets) Label() string {
	return "Loading secrets"
}

func (s *Secrets) Run(rc *RunContext, next Next) error {
	rc.Secrets.Store = secrets.InMemoryStore()
	for k, v := range envMap(s.opts.Secrets) {
		rc.Secrets.Store.AddSecret(rc, k, []byte(v))
	}

	rc.Display.Stdout = rc.Secrets.Store.Pipe(rc, rc.Display.Stdout, []byte("***"))
	var isTerm = term.IsTerminal(int(os.Stdout.Fd()))

	if isTerm {
		rc.Display.Stderr = rc.Display.Stdout
	} else {
		rc.Display.Stderr = rc.Secrets.Store.Pipe(rc, rc.Display.Stderr, []byte("***"))
	}

	return next(rc)
}
