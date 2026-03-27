package run

import (
	"os"
	"strings"

	"github.com/spf13/pflag"
)

type EnvsOptions struct {
	Envs []string
}

func (s *EnvsOptions) BindFlags(flags *pflag.FlagSet) {
	flags.StringSliceVarP(&s.Envs, "env", "e", s.Envs, "Pass envs to the pipeline.")
}

func (s EnvsOptions) Build() Step {
	return &Envs{opts: s}
}

type Envs struct {
	opts EnvsOptions
}

type EnvsContext struct {
	Envs map[string]string
}

func (s *Envs) Run(rc *RunContext, next Next) error {
	rc.Envs.Envs = envMap(s.opts.Envs)
	return next(rc)
}

func envMap(from []string) map[string]string {
	envs := make(map[string]string)
	for _, v := range from {
		parts := strings.SplitN(v, "=", 2)
		if len(parts) == 1 {
			if env, ok := os.LookupEnv(parts[0]); ok {
				envs[parts[0]] = env
			}
		} else {
			envs[parts[0]] = parts[1]
		}
	}
	return envs
}

func osEnvMap() map[string]string {
	envs := make(map[string]string)
	for _, v := range os.Environ() {
		parts := strings.SplitN(v, "=", 2)
		if len(parts) == 2 {
			envs[parts[0]] = parts[1]
		}
	}
	return envs
}
