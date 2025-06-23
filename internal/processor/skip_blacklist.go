package processor

import (
	"errors"
	"slices"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithSkipBlacklist(blacklist []string) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if len(blacklist) == 0 {
			return nil
		}

		return &SkipBlacklist{
			stepName:  spec.Name,
			blacklist: blacklist,
		}
	}
}

type SkipBlacklist struct {
	stepName  string
	blacklist []string
}

var ErrSkipBlacklist = errors.New("skip blacklisted step")

func (s *SkipBlacklist) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		if !slices.Contains(s.blacklist, s.stepName) {
			return next(ctx)
		}

		return ctx, ErrSkipBlacklist
	}, nil
}
