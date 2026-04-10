// Package tracing provides OpenTelemetry initialization for nix-key.
//
// When otelEndpoint is non-nil, it creates a tracer provider with an
// OTLP gRPC exporter. When nil, the global no-op tracer is used (zero overhead).
package tracing

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

const tracerName = "github.com/phaedrus-raznikov/nix-key"

// Provider wraps the OTEL tracer provider and exposes a Tracer for creating spans.
type Provider struct {
	tp     *sdktrace.TracerProvider // nil when no-op
	tracer trace.Tracer
}

// Init creates a new Provider. If endpoint is nil or empty, a no-op tracer
// is returned with zero overhead. Otherwise, an OTLP gRPC exporter is
// configured to send traces to the given endpoint (e.g. "localhost:4317").
func Init(ctx context.Context, endpoint *string) (*Provider, error) {
	if endpoint == nil || *endpoint == "" {
		return &Provider{
			tracer: noop.NewTracerProvider().Tracer(tracerName),
		}, nil
	}

	exp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(*endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("creating OTLP exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("nix-key"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp, sdktrace.WithBatchTimeout(2*time.Second)),
		sdktrace.WithResource(res),
	)

	// Set W3C trace context propagator so otelgrpc handlers inject/extract
	// traceparent headers for distributed tracing across host and phone.
	otel.SetTextMapPropagator(propagation.TraceContext{})
	otel.SetTracerProvider(tp)

	return &Provider{
		tp:     tp,
		tracer: tp.Tracer(tracerName),
	}, nil
}

// InitWithExporter creates a Provider using the given SpanExporter.
// This is useful for testing with in-memory exporters.
func InitWithExporter(exp sdktrace.SpanExporter) *Provider {
	res := resource.NewSchemaless(
		semconv.ServiceName("nix-key"),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exp),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)

	return &Provider{
		tp:     tp,
		tracer: tp.Tracer(tracerName),
	}
}

// Tracer returns the tracer for creating spans.
func (p *Provider) Tracer() trace.Tracer {
	return p.tracer
}

// Shutdown flushes and shuts down the tracer provider.
// No-op if the provider was initialized without an endpoint.
func (p *Provider) Shutdown(ctx context.Context) error {
	if p.tp == nil {
		return nil
	}
	return p.tp.Shutdown(ctx)
}
