package run

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/pflag"
)

type LifecycleOptions struct {
	Timeout time.Duration
}

func (s LifecycleOptions) Build() Step {
	return &Lifecycle{
		opts: s,
	}
}

func (s *LifecycleOptions) BindFlags(flags *pflag.FlagSet) {
	flags.DurationVarP(&s.Timeout, "timeout", "", 0, "")
}

type Lifecycle struct {
	opts LifecycleOptions
}

func (s *Lifecycle) Run(rc *RunContext, next Next) error {
	ctx, cancel := context.WithCancel(rc.Context)
	rc.Context = ctx

	if s.opts.Timeout > 0 {
		ctx, c := context.WithTimeout(rc.Context, s.opts.Timeout)
		rc.Context = ctx
		cancel = c
	}

	defer cancel()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-signals
		rc.Logging.Logger.V(1).Info("received signal", "signal", sig)
		cancel()
	}()

	return next(rc)
}
