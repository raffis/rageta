package processor

import (
	"github.com/go-logr/logr"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"go.opentelemetry.io/otel/attribute"
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
		var span trace.Span
		ctx.Context, span = s.tracer.Start(ctx, s.stepName, trace.WithSpanKind(trace.SpanKindInternal))
		defer span.End()

		for _, tag := range ctx.Tags() {
			span.SetAttributes(attribute.String(tag.Key, tag.Value))
		}

		ctx.Context = logr.NewContext(ctx, logr.FromContextOrDiscard(ctx).WithValues(
			"step", s.stepName,
			"span-id", span.SpanContext().SpanID(),
			"trace-id", span.SpanContext().TraceID()),
		)

		return next(ctx)
	}, nil
}
