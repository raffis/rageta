package run

import (
	"os"
	"slices"

	"github.com/raffis/rageta/internal/setup/flagset"
)

type ForkOptions struct {
	Fork bool
}

func (s ForkOptions) Build() Task {
	return &Fork{
		opts: s,
	}
}

func (s *ForkOptions) BindFlags(flags flagset.Interface) {
	flags.BoolVarP(&s.Fork, "fork", "", s.Fork, "Creates a controller container which handles this pipeline and exit.")
}

type Fork struct {
	opts ForkOptions
}

func (s *Fork) Run(rc *RunContext, next Next) error {
	if !s.opts.Fork {
		return next(rc)
	}

	rc.Logging.Logger.V(0).Info("fork pipeline runner, attaching streams. This process can be exited using ctrl+c")

	forkFlags := os.Args[1:]
	forkFlags = slices.DeleteFunc(forkFlags, func(v string) bool { return v == "--fork" })

	/*container := cruntime.ContainerSpec{
		Name:  "rageta",
		Image: "ghcr.io/rageta/rageta:latest",
		Args:  forkFlags,
		Stdin: true,
		//TTY:             IsTerm(),
		Env:             rc.Envs.Envs,
		ImagePullPolicy: rc.ImagePolicy.PullPolicy,
	}*/

	return nil
	//return status.Wait(rc)
}
