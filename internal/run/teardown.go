package run

import (
	"context"
	"sync"
	"time"

	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/setup/flagset"
)

type TeardownOptions struct {
	GracePeriod time.Duration
}

func (s *TeardownOptions) BindFlags(flags flagset.Interface) {
	flags.DurationVarP(&s.GracePeriod, "grace-period", "", s.GracePeriod, "Maximum time to wait for termination and cleanup of steps.")
}

func NewTeardownOptions() TeardownOptions {
	return TeardownOptions{
		GracePeriod: time.Second * 10,
	}
}

func (s TeardownOptions) Build() Task {
	return &Teardown{opts: s}
}

type Teardown struct {
	opts TeardownOptions
}

type TeardownContext struct {
	Teardown chan processor.Teardown
}

func (s *Teardown) Label() string {
	return "Setting up teardown"
}

func (s *Teardown) Run(rc *RunContext, next Next) error {
	teardown := make(chan processor.Teardown)
	rc.Teardown.Teardown = teardown

	var stack []processor.Teardown

	go func() {
		for fn := range rc.Teardown.Teardown {
			stack = append(stack, fn)
		}
	}()

	err := next(rc)
	close(teardown)
	s.runTeardown(rc, stack)
	return err
}

func (s *Teardown) runTeardown(rc *RunContext, stack []processor.Teardown) {
	wg := &sync.WaitGroup{}

	for _, fn := range stack {
		wg.Add(1)
		go func(fn processor.Teardown) {
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

	wg.Wait()
}
