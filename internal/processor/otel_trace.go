package processor

import (
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
	return func(ctx StepContext) (StepContext, error) {
		traceCtx, span := s.tracer.Start(ctx, s.stepName)
		defer span.End()
		ctx.Context = traceCtx

		ctx.Context = logr.NewContext(ctx, logr.FromContextOrDiscard(ctx).WithValues(
			"step", s.stepName,
			"span-id", span.SpanContext().SpanID(),
			"trace-id", span.SpanContext().TraceID()),
		)

		/*
		   logger.Info("process step")
		   logger.V(1).Info("step context input", "context", ctx)
		   ctx, err := next(ctx)
		   logger.Info("process step done", "err", err)
		   logger.V(1).Info("step done", "err", err, "context", ctx)
		*/
		return next(ctx)
	}, nil
}
