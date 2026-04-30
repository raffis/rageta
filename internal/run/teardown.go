package run

import (
	"context"
	"sync"
	"time"

	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/setup/flagset"
)

type TeardownOptions struct {
	Disabled    bool
	GracePeriod time.Duration
}

func (s *TeardownOptions) BindFlags(flags flagset.Interface) {
	flags.BoolVarP(&s.Disabled, "skip-gc", "", s.Disabled, "Keep all containers and temporary files after execution.")
	flags.DurationVarP(&s.GracePeriod, "grace-period", "", s.GracePeriod, "Maximum time to wait for termination and cleanup of steps.")
}

func NewTeardownOptions() TeardownOptions {
	return TeardownOptions{
		GracePeriod: time.Second * 10,
	}
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
	wg := &sync.WaitGroup{}

	defer func() {
		wg.Wait()
	}()

	go func() {
		s.runTeardown(rc, wg)
	}()

	return next(rc)
}

func (s *Teardown) runTeardown(rc *RunContext, wg *sync.WaitGroup) {
	for fn := range rc.Teardown.Teardown {
		go func(fn processor.Teardown) {
			wg.Add(1)
			defer wg.Done()

			teardownCtx := context.TODO()
			if s.opts.GracePeriod > 0 {
				ctx, cancel := context.WithTimeout(teardownCtx, s.opts.GracePeriod)
				teardownCtx = ctx
				defer cancel()
			}

			rc.Logging.Logger.V(5).Info("execute teardown")
			if err := fn(teardownCtx, s.opts.GracePeriod); err != nil {
				rc.Logging.Logger.V(5).Info("failed execute teardown", "err", err)
			}
		}(fn)
	}
}
