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

func (s LifecycleOptions) BindFlags(flags *pflag.FlagSet) {
	flags.DurationVarP(&s.Timeout, "timeout", "", 0, "")
}

type Lifecycle struct {
	opts LifecycleOptions
}

func (s *Lifecycle) Run(rc *RunContext, next Next) error {
	ctx, cancel := context.WithCancel(rc.Input.Ctx)
	rc.Ctx = ctx
	rc.Cancel = cancel

	if rc.Input.Timeout > 0 {
		ctx, cancel := context.WithTimeout(rc.Ctx, rc.Input.Timeout)
		rc.Ctx = ctx
		rc.Cancel = cancel
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-signals
		rc.Logger.V(1).Info("received signal", "signal", sig)
		rc.Cancel()
	}()

	return next(rc)
}
