package run

import (
	"context"
	"time"

	"github.com/raffis/rageta/internal/processor"
	"github.com/spf13/pflag"
)

type TeardownOptions struct {
	MaxConcurrent int
	Disabled      bool
}

func (s *TeardownOptions) BindFlags(flags *pflag.FlagSet) {
	flags.BoolVarP(&s.Disabled, "no-gc", "", s.Disabled, "Keep all containers and temporary files after execution.")
}

func (s TeardownOptions) Build() Step {
	return &Teardown{opts: s}
}

type Teardown struct {
	opts TeardownOptions
}

type TeardownContext struct {
	Teardown chan processor.Teardown
	Enabled  bool
}

func (s *Teardown) Run(rc *RunContext, next Next) error {
	teardown := make(chan processor.Teardown)
	rc.Teardown.Teardown = teardown
	rc.Teardown.Enabled = !s.opts.Disabled

	defer close(teardown)

	go func() {
		s.runTeardown(rc)
	}()

	return next(rc)
}

func (s *Teardown) runTeardown(rc *RunContext) {
	/*teardownCtx, cancel := context.WithTimeout(context.Background(), s.opts.GracefulTermination+time.Second)
	defer cancel()*/

	for fn := range rc.Teardown.Teardown {
		go func(fn processor.Teardown) {
			teardownCtx := context.TODO()

			rc.Logging.Logger.V(5).Info("execute teardown")
			if err := fn(teardownCtx, time.Second*2); err != nil {
				rc.Logging.Logger.V(5).Info("failed execute teardown", "err", err)
			}
		}(fn)
	}
}
