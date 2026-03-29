package phoneserver_test

import (
	"context"
	"net"
	"testing"
	"time"

	nixkeyv1 "github.com/phaedrus-raznikov/nix-key/gen/nixkey/v1"
	"github.com/phaedrus-raznikov/nix-key/pkg/phoneserver"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// startOTELTestServer creates a phoneserver with OTEL tracing configured,
// starts it on a gRPC server with the otelgrpc server handler, and returns
// a client that also has the otelgrpc client handler for traceparent injection.
func startOTELTestServer(t *testing.T, ks phoneserver.KeyStore, conf phoneserver.Confirmer, tp *trace.TracerProvider) (nixkeyv1.NixKeyAgentClient, func()) {
	t.Helper()

	// Ensure W3C trace context propagation is configured for otelgrpc.
	otel.SetTextMapPropagator(propagation.TraceContext{})

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := phoneserver.NewServerWithTracing(ks, conf, tp)
	gs := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler(otelgrpc.WithTracerProvider(tp))),
	)
	nixkeyv1.RegisterNixKeyAgentServer(gs, srv)
	go func() { _ = gs.Serve(lis) }()

	conn, err := grpc.NewClient(
		lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler(otelgrpc.WithTracerProvider(tp))),
	)
	if err != nil {
		gs.GracefulStop()
		t.Fatalf("dial: %v", err)
	}

	client := nixkeyv1.NewNixKeyAgentClient(conn)

	cleanup := func() {
		conn.Close()
		gs.GracefulStop()
	}

	return client, cleanup
}

// TestOTELSignSpans verifies that a gRPC Sign call with traceparent header
// produces the expected child spans with correct parent-child relationships.
func TestOTELSignSpans(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(trace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())

	ks := &mockKeyStore{keys: testKeys()}
	client, cleanup := startOTELTestServer(t, ks, &autoApproveConfirmer{}, tp)
	defer cleanup()

	// Create a client-side root span to simulate the host's ssh-sign-request.
	tracer := tp.Tracer("otel-test-client")
	ctx, rootSpan := tracer.Start(context.Background(), "client-root")

	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := client.Sign(callCtx, &nixkeyv1.SignRequest{
		KeyFingerprint: "SHA256:abc123",
		Data:           []byte("otel test data"),
		Flags:          0,
	})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if len(resp.Signature) == 0 {
		t.Fatal("expected non-empty signature")
	}

	rootSpan.End()

	// Collect all spans.
	spans := exporter.GetSpans()
	spansByName := make(map[string]tracetest.SpanStub)
	for _, s := range spans {
		spansByName[s.Name] = s
	}

	// Verify required phone-side spans exist.
	requiredSpans := []string{"handle-sign-request", "show-prompt", "user-response", "keystore-sign"}
	for _, name := range requiredSpans {
		if _, ok := spansByName[name]; !ok {
			t.Errorf("missing expected span %q; got spans: %v", name, allSpanNames(spans))
		}
	}

	// Verify all phone-side spans share the same trace ID as the client root span.
	rootTraceID := rootSpan.SpanContext().TraceID()
	for _, name := range requiredSpans {
		s, ok := spansByName[name]
		if !ok {
			continue
		}
		if s.SpanContext.TraceID() != rootTraceID {
			t.Errorf("span %q trace ID %s != root trace ID %s", name, s.SpanContext.TraceID(), rootTraceID)
		}
	}

	// Verify handle-sign-request is a child of the gRPC server span (which is a
	// child of the client root span). The otelgrpc server handler creates a span
	// named like "nixkey.v1.NixKeyAgent/Sign". handle-sign-request should be a
	// child of that span.
	handleSpan, ok := spansByName["handle-sign-request"]
	if !ok {
		t.Fatal("handle-sign-request span not found")
	}

	// Verify child spans are children of handle-sign-request.
	handleSpanID := handleSpan.SpanContext.SpanID()
	childSpans := []string{"show-prompt", "user-response", "keystore-sign"}
	for _, name := range childSpans {
		s, ok := spansByName[name]
		if !ok {
			continue
		}
		if s.Parent.SpanID() != handleSpanID {
			t.Errorf("span %q parent span ID %s != handle-sign-request span ID %s",
				name, s.Parent.SpanID(), handleSpanID)
		}
	}
}

// TestOTELSignSpansWithDenial verifies spans are created even when user denies.
func TestOTELSignSpansWithDenial(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(trace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())

	ks := &mockKeyStore{keys: testKeys()}
	client, cleanup := startOTELTestServer(t, ks, &denyConfirmer{}, tp)
	defer cleanup()

	tracer := tp.Tracer("otel-test-client")
	ctx, rootSpan := tracer.Start(context.Background(), "client-root-deny")

	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := client.Sign(callCtx, &nixkeyv1.SignRequest{
		KeyFingerprint: "SHA256:abc123",
		Data:           []byte("deny test"),
		Flags:          0,
	})
	if err == nil {
		t.Fatal("expected error for denied sign")
	}

	rootSpan.End()

	spans := exporter.GetSpans()
	spansByName := make(map[string]tracetest.SpanStub)
	for _, s := range spans {
		spansByName[s.Name] = s
	}

	// show-prompt and user-response should still be created on denial.
	if _, ok := spansByName["handle-sign-request"]; !ok {
		t.Errorf("missing handle-sign-request span on denial; got: %v", allSpanNames(spans))
	}
	if _, ok := spansByName["show-prompt"]; !ok {
		t.Errorf("missing show-prompt span on denial; got: %v", allSpanNames(spans))
	}
	if _, ok := spansByName["user-response"]; !ok {
		t.Errorf("missing user-response span on denial; got: %v", allSpanNames(spans))
	}

	// user-response should record the denial.
	if respSpan, ok := spansByName["user-response"]; ok {
		found := false
		for _, attr := range respSpan.Attributes {
			if string(attr.Key) == "user.approved" && !attr.Value.AsBool() {
				found = true
				break
			}
		}
		if !found {
			t.Error("user-response span should have user.approved=false attribute")
		}
	}

	// keystore-sign should NOT be present when user denies.
	if _, ok := spansByName["keystore-sign"]; ok {
		t.Error("keystore-sign span should not exist when user denies")
	}
}

// TestOTELNoopWhenNoTracer verifies the server works without tracing configured.
func TestOTELNoopWhenNoTracer(t *testing.T) {
	ks := &mockKeyStore{keys: testKeys()}
	// Use NewServer (no tracing) — should work without any OTEL overhead.
	client, cleanup := startTestServer(t, ks, &autoApproveConfirmer{})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.Sign(ctx, &nixkeyv1.SignRequest{
		KeyFingerprint: "SHA256:abc123",
		Data:           []byte("noop test"),
		Flags:          0,
	})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if len(resp.Signature) == 0 {
		t.Fatal("expected non-empty signature")
	}
}

func allSpanNames(spans []tracetest.SpanStub) []string {
	names := make([]string, len(spans))
	for i, s := range spans {
		names[i] = s.Name
	}
	return names
}
