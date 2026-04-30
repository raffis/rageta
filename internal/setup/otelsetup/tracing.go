package otelsetup

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
	"google.golang.org/grpc/credentials"
)

func (o *Options) BuildTraceProvider(ctx context.Context) (*trace.TracerProvider, error) {
	var providers []trace.TracerProviderOption

	if o.Endpoint != "" {
		var grpcOptions []otlptracegrpc.Option

		if o.Endpoint != "" {
			grpcOptions = append(grpcOptions, otlptracegrpc.WithEndpoint(o.Endpoint))
		}

		if o.Insecure {
			grpcOptions = append(grpcOptions, otlptracegrpc.WithInsecure())
		} else {
			tlso, err := o.getTLSConfig()
			if err != nil {
				return nil, err
			}

			grpcOptions = append(grpcOptions, otlptracegrpc.WithTLSCredentials(credentials.NewTLS(tlso)))

		}

		exporter, err := otlptracegrpc.New(
			ctx,
			grpcOptions...,
		)

		if err != nil {
			return nil, err
		}

		providers = append(providers, trace.WithBatcher(exporter))
	}

	if o.Stdout {
		exporter, err := stdouttrace.New()
		if err != nil {
			return nil, err
		}

		providers = append(providers, trace.WithBatcher(exporter))
	}

	if len(providers) == 0 {
		return nil, nil
	}

	// labels/tags/resources that are common to all traces.
	providers = append(providers, trace.WithResource(resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String(o.ServiceName),
	)))

	providers = append(providers, trace.WithSampler(trace.ParentBased(trace.TraceIDRatioBased(1))))
	provider := trace.NewTracerProvider(providers...)

	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{}, // W3C Trace Context format; https://www.w3.org/TR/trace-context/
		),
	)

	return provider, nil
}
