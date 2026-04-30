package otelsetup

import (
	"context"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
	"google.golang.org/grpc/credentials"
)

// BuildLoggerProvider creates a LoggerProvider from options (same flags as tracing/metrics).
// When no export is configured (no endpoint, no stdout), returns (nil, nil).
func (o *Options) BuildLoggerProvider(ctx context.Context) (*sdklog.LoggerProvider, error) {
	var processors []sdklog.Processor

	if o.Endpoint != "" {
		grpcOptions := []otlploggrpc.Option{
			otlploggrpc.WithEndpoint(o.Endpoint),
		}
		if o.Insecure {
			grpcOptions = append(grpcOptions, otlploggrpc.WithInsecure())
		} else {
			tlso, err := o.getTLSConfig()
			if err != nil {
				return nil, err
			}
			grpcOptions = append(grpcOptions, otlploggrpc.WithTLSCredentials(credentials.NewTLS(tlso)))
		}

		exporter, err := otlploggrpc.New(ctx, grpcOptions...)
		if err != nil {
			return nil, err
		}
		processors = append(processors, sdklog.NewBatchProcessor(exporter))
	}

	if o.Stdout {
		exporter, err := stdoutlog.New()
		if err != nil {
			return nil, err
		}
		processors = append(processors, sdklog.NewBatchProcessor(exporter))
	}

	if len(processors) == 0 {
		return nil, nil
	}

	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String(o.ServiceName),
	)

	opts := []sdklog.LoggerProviderOption{sdklog.WithResource(res)}
	for _, p := range processors {
		opts = append(opts, sdklog.WithProcessor(p))
	}

	return sdklog.NewLoggerProvider(opts...), nil
}
