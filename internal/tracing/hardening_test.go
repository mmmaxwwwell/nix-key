package tracing

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

// T-HI-09a: Tracing disabled = no overhead — when endpoint is nil or empty,
// the provider uses a no-op tracer with zero runtime cost.
func TestIntegrationTracingNoOverheadWhenDisabled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

	t.Run("NilEndpoint", func(t *testing.T) {
		p, err := Init(ctx, nil)
		if err != nil {
			t.Fatalf("Init with nil endpoint: %v", err)
		}
		// tp should be nil (no-op provider).
		if p.tp != nil {
			t.Error("tp should be nil when endpoint is nil")
		}

		tracer := p.Tracer()
		if tracer == nil {
			t.Fatal("Tracer() should not return nil even when disabled")
		}

		// Creating spans should be no-ops.
		_, span := tracer.Start(ctx, "test-span")
		if span.SpanContext().IsValid() {
			t.Error("no-op span should have invalid SpanContext")
		}
		span.End() // should not panic

		// Shutdown should be a no-op.
		if err := p.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown with nil provider: %v", err)
		}
	})

	t.Run("EmptyEndpoint", func(t *testing.T) {
		empty := ""
		p, err := Init(ctx, &empty)
		if err != nil {
			t.Fatalf("Init with empty endpoint: %v", err)
		}
		if p.tp != nil {
			t.Error("tp should be nil when endpoint is empty")
		}

		tracer := p.Tracer()
		_, span := tracer.Start(ctx, "empty-span")
		if span.SpanContext().IsValid() {
			t.Error("no-op span should have invalid SpanContext")
		}
		span.End()

		if err := p.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})

	t.Run("NoOpTracerType", func(t *testing.T) {
		p, err := Init(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}

		tracer := p.Tracer()

		// Verify the tracer produces non-recording spans (no-op behavior).
		_, span := tracer.Start(ctx, "type-check")
		if span.IsRecording() {
			t.Error("no-op tracer should produce non-recording spans")
		}
		span.End()

		// Adding attributes to a no-op span should not panic.
		_, span2 := tracer.Start(ctx, "attrs")
		span2.SetAttributes() // no args, no panic
		span2.AddEvent("test-event")
		span2.End()
	})

	t.Run("DisabledVsEnabled", func(t *testing.T) {
		// Disabled: nil endpoint.
		disabledP, err := Init(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		disabledTracer := disabledP.Tracer()

		// Verify disabled tracer interface matches expectations.
		var _ trace.Tracer = disabledTracer

		_, disabledSpan := disabledTracer.Start(ctx, "check")
		if disabledSpan.IsRecording() {
			t.Error("disabled span should not be recording")
		}
		if disabledSpan.SpanContext().IsValid() {
			t.Error("disabled span context should be invalid")
		}
		disabledSpan.End()

		if err := disabledP.Shutdown(ctx); err != nil {
			t.Errorf("disabled shutdown: %v", err)
		}
	})
}
