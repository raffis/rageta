package processor

import (
	"time"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	stepDurationName        = "rageta.step.duration"
	stepDurationDescription = "Step execution duration in seconds"
	stepDurationUnit        = "s"
)

func WithOtelMetrics(meter metric.Meter) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if meter == nil {
			return nil
		}

		return &OtelMetrics{
			stepName: spec.Name,
			meter:    meter,
		}
	}
}

type OtelMetrics struct {
	stepName string
	meter    metric.Meter
}

func (s *OtelMetrics) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	histogram, err := s.meter.Float64Histogram(stepDurationName,
		metric.WithDescription(stepDurationDescription),
		metric.WithUnit(stepDurationUnit))
	if err != nil {
		return nil, err
	}

	return func(ctx StepContext) (StepContext, error) {
		start := time.Now()
		ctx, err := next(ctx)
		duration := time.Since(start).Seconds()

		attrs := []attribute.KeyValue{
			attribute.String("step", s.stepName),
			attribute.String("result", ErrorResult(err)),
		}
		for _, tag := range ctx.Tags() {
			attrs = append(attrs, attribute.String(tag.Key, tag.Value))
		}

		histogram.Record(ctx, duration, metric.WithAttributes(attrs...))
		return ctx, err
	}, nil
}
