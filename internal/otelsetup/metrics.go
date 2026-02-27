package otelsetup

import (
	"context"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
	"google.golang.org/grpc/credentials"
)

// BuildMeterProvider creates a MeterProvider from options (same flags as tracing).
// When no export is configured (no endpoint, no stdout), returns (nil, nil) and
// the caller should pass a nil meter so metrics are not collected.
func (o *Options) BuildMeterProvider(ctx context.Context) (*metric.MeterProvider, error) {
	var readers []metric.Reader

	if o.Endpoint != "" {
		grpcOptions := []otlpmetricgrpc.Option{
			otlpmetricgrpc.WithEndpoint(o.Endpoint),
		}
		if o.Insecure {
			grpcOptions = append(grpcOptions, otlpmetricgrpc.WithInsecure())
		} else {
			tlso, err := o.getTLSConfig()
			if err != nil {
				return nil, err
			}
			grpcOptions = append(grpcOptions, otlpmetricgrpc.WithTLSCredentials(credentials.NewTLS(tlso)))
		}

		exporter, err := otlpmetricgrpc.New(ctx, grpcOptions...)
		if err != nil {
			return nil, err
		}
		readers = append(readers, metric.NewPeriodicReader(exporter))
	}

	if o.Stdout {
		exporter, err := stdoutmetric.New()
		if err != nil {
			return nil, err
		}
		readers = append(readers, metric.NewPeriodicReader(exporter))
	}

	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String(o.ServiceName),
	)

	opts := []metric.Option{metric.WithResource(res)}
	for _, r := range readers {
		opts = append(opts, metric.WithReader(r))
	}

	return metric.NewMeterProvider(opts...), nil
}
