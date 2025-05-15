package processor

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithDebug(logger logr.Logger, debug bool, spec *v1beta1.Step, processors ...Bootstraper) []Bootstraper {
	if !debug {
		return processors
	}

	for k, v := range processors {
		logger.V(1).Info("step", "spec", spec)

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
	logger.V(1).Info("register step processor")
	wrappedNext, err := s.wrapped.Bootstrap(pipeline, next)
	if err != nil {
		return next, err
	}

	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		logger.V(1).Info("pre processor", "context", stepContext)
		stepContext, err := wrappedNext(ctx, stepContext)
		logger.V(1).Info("post processor", "context", stepContext, "err", err)

		return stepContext, err
	}, nil
}
