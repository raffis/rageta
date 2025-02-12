package processor

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"go.opentelemetry.io/otel/trace"
)

func WithOtelTrace(logger logr.Logger, tracer trace.Tracer) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if tracer == nil {
			return nil
		}

		return &OtelTrace{
			stepName: spec.Name,
			logger:   logger,
			tracer:   tracer,
		}
	}
}

type OtelTrace struct {
	stepName string
	logger   logr.Logger
	tracer   trace.Tracer
}

func (s *OtelTrace) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		ctx, span := s.tracer.Start(ctx, s.stepName)
		defer span.End()

		logger := s.logger.WithValues(
			"step", s.stepName,
			"span-id", span.SpanContext().SpanID(),
			"trace-id", span.SpanContext().TraceID())

		logger.Info("process step")
		logger.V(1).Info("step context input", "context", stepContext)
		stepContext, err := next(ctx, stepContext)
		logger.Info("process step done", "err", err)
		logger.V(1).Info("step done", "err", err, "context", stepContext)

		return stepContext, err
	}, nil
}
