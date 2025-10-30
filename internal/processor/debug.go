package processor

import (
	"fmt"

	"github.com/go-logr/logr"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithDebug(logger logr.Logger, debug bool, spec *v1beta1.Step, processors ...Bootstraper) []Bootstraper {
	if !debug {
		return processors
	}
	logger.V(5).Info("step", "spec", spec)

	for k, v := range processors {

		processors[k] = &Debug{
			spec:    spec,
			logger:  logger,
			wrapped: v,
		}
	}

	return processors
}

type Debug struct {
	spec    *v1beta1.Step
	logger  logr.Logger
	wrapped Bootstraper
}

func (s *Debug) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	logger := s.logger.WithValues("step", s.spec.Name, "processor", fmt.Sprintf("%T", s.wrapped))
	logger.V(7).Info("register step processor")
	wrappedNext, err := s.wrapped.Bootstrap(pipeline, next)
	if err != nil {
		return next, err
	}

	return func(ctx StepContext) (StepContext, error) {
		logger.V(6).Info("pre processor", "context", ctx)
		ctx, err := wrappedNext(ctx)
		logger.V(6).Info("post processor", "context", ctx, "err", err)

		return ctx, err
	}, nil
}
