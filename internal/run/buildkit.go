package run

import (
	"github.com/moby/buildkit/client"
	"github.com/spf13/pflag"
)

func NewBuildkitOptions() BuildkitOptions {
	return BuildkitOptions{}
}

type BuildkitOptions struct {
}

func (s BuildkitOptions) Build() Step {
	return &Buildkit{}
}

func (s BuildkitOptions) BindFlags(flags *pflag.FlagSet) {
}

type Buildkit struct {
	opts BuildkitOptions
}

type BuildkitContext struct {
	Client *client.Client
}

func (s *Buildkit) Run(rc *RunContext, next Next) error {
	c, err := client.New(rc, "tcp://172.17.0.2:1234")
	if err != nil {
		return err
	}

	rc.Buildkit.Client = c
	return next(rc)
}
