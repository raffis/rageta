package run

import (
	"os"
)

type Step interface {
	Run(rc *RunContext, next Next) error
}

type Next func(rc *RunContext) error

type Runner struct {
	steps []RunnerStep
}

func Builder(steps ...RunnerStep) *Runner {
	result := &Runner{}
	result.steps = steps
	return result
}

func (r *Runner) Run(in RunInput) (rc *RunContext, err error) {
	if in.Stdout == nil {
		in.Stdout = os.Stdout
	}
	if in.Stderr == nil {
		in.Stderr = os.Stderr
	}

	rc = &RunContext{
		Input:  &in,
		Stdout: in.Stdout,
		Stderr: in.Stderr,
	}

	defer func() {
		if rc.PersistDB != nil {
			_ = rc.PersistDB()
		}
	}()

	noop := func(rc *RunContext) error { return nil }
	chain := noop
	for i := len(r.steps) - 1; i >= 0; i-- {
		step := r.steps[i]
		next := chain
		chain = func(rc *RunContext) error {
			return step.Run(rc, next)
		}
	}
	err = chain(rc)
	return rc, err
}
