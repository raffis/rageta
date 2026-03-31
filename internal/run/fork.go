package run

import (
	"fmt"
	"os"
	"slices"

	cruntime "github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/internal/utils"
	"github.com/spf13/pflag"
)

type ForkOptions struct {
	Fork bool
}

func (s ForkOptions) Build() Step {
	return &Fork{
		opts: s,
	}
}

func (s *ForkOptions) BindFlags(flags *pflag.FlagSet) {
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

	container := cruntime.ContainerSpec{
		Name:  "rageta",
		Image: "ghcr.io/rageta/rageta:latest",
		Args:  forkFlags,
		Stdin: true,
		//TTY:             IsTerm(),
		Env:             rc.Envs.Envs,
		ImagePullPolicy: rc.ImagePolicy.PullPolicy,
	}
	pod := cruntime.Pod{
		Name: fmt.Sprintf("rageta-%s", utils.RandString(5)),
		Spec: cruntime.PodSpec{
			Containers: []cruntime.ContainerSpec{container},
		},
	}

	status, err := rc.ContainerRuntime.Driver.CreatePod(rc.Context, &pod, os.Stdin, rc.Output.Stdout, rc.Output.Stderr)
	if err != nil {
		return err
	}

	/*if !s.noGC {
		defer func() {
			_ = rc.Driver.DeletePod(rc.Ctx, &pod, s.gracefulTermination)
		}()
	}*/

	return status.Wait(rc)
}
