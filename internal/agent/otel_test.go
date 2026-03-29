package agent_test

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"golang.org/x/crypto/ssh"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/phaedrus-raznikov/nix-key/internal/agent"
	"github.com/phaedrus-raznikov/nix-key/internal/daemon"
	nixkeyv1 "github.com/phaedrus-raznikov/nix-key/gen/nixkey/v1"
)

// otelTestDialer wraps testDialer but adds otelgrpc stats handler for
// W3C traceparent injection into gRPC metadata.
type otelTestDialer struct {
	dialErr error
}

func (d *otelTestDialer) DialDevice(ctx context.Context, dev daemon.Device) (nixkeyv1.NixKeyAgentClient, func(), error) {
	if d.dialErr != nil {
		return nil, nil, d.dialErr
	}

	addr := fmt.Sprintf("%s:%d", dev.TailscaleIP, dev.ListenPort)
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		return nil, nil, err
	}

	client := nixkeyv1.NewNixKeyAgentClient(conn)
	cleanup := func() { conn.Close() }
	return client, cleanup, nil
}

// TestIntegrationOTELSignRequestSpans verifies that a sign request produces
// the expected OTEL spans with correct parent-child relationships.
func TestIntegrationOTELSignRequestSpans(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Set up in-memory span exporter as mock collector.
	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(trace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("nix-key-test")

	_, signer, grpcKey := newTestECDSAKey(t, "otel-key")
	phone := &testPhoneServer{
		keys:   []*nixkeyv1.SSHKey{grpcKey},
		signer: signer,
	}

	addr, cleanup := startTestPhoneServer(t, phone)
	t.Cleanup(cleanup)

	host, portStr, _ := net.SplitHostPort(addr)
	port := 0
	fmt.Sscanf(portStr, "%d", &port)

	registry := daemon.NewRegistry()
	registry.Add(daemon.Device{
		ID:              "otel-device",
		Name:            "OTEL Phone",
		TailscaleIP:     host,
		ListenPort:      port,
		CertFingerprint: "otel-fp",
		Source:          daemon.SourceRuntimePaired,
	})

	backend := agent.NewGRPCBackend(agent.GRPCBackendConfig{
		Registry:          registry,
		Dialer:            &otelTestDialer{},
		AllowKeyListing:   true,
		ConnectionTimeout: 5 * time.Second,
		SignTimeout:       5 * time.Second,
		Tracer:            tracer,
	})

	// List keys to populate cache.
	keys, err := backend.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}

	// Clear any spans from the list operation.
	exporter.Reset()

	// Perform sign request.
	pub, err := ssh.ParsePublicKey(grpcKey.GetPublicKeyBlob())
	if err != nil {
		t.Fatalf("parse public key: %v", err)
	}

	sig, err := backend.Sign(pub, []byte("otel test data"), 0)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if sig == nil {
		t.Fatal("signature is nil")
	}

	// Verify signature.
	if err := pub.Verify([]byte("otel test data"), sig); err != nil {
		t.Fatalf("verify: %v", err)
	}

	// Check spans.
	spans := exporter.GetSpans()

	// Build a map of span names for verification.
	spanNames := make(map[string]tracetest.SpanStub)
	for _, s := range spans {
		spanNames[s.Name] = s
	}

	// Verify required spans exist.
	requiredSpans := []string{"ssh-sign-request", "device-lookup", "mtls-connect", "return-signature"}
	for _, name := range requiredSpans {
		if _, ok := spanNames[name]; !ok {
			t.Errorf("missing expected span %q; got spans: %v", name, spanNameList(spans))
		}
	}

	// Verify parent-child relationships.
	rootSpan, ok := spanNames["ssh-sign-request"]
	if !ok {
		t.Fatal("root span ssh-sign-request not found")
	}
	rootSpanID := rootSpan.SpanContext.SpanID()
	rootTraceID := rootSpan.SpanContext.TraceID()

	childSpans := []string{"device-lookup", "mtls-connect", "return-signature"}
	for _, name := range childSpans {
		child, ok := spanNames[name]
		if !ok {
			continue // already reported above
		}
		// Child must have same trace ID as root.
		if child.SpanContext.TraceID() != rootTraceID {
			t.Errorf("span %q has different trace ID than root", name)
		}
		// Child's parent must be the root span.
		if child.Parent.SpanID() != rootSpanID {
			t.Errorf("span %q parent span ID %s != root span ID %s",
				name, child.Parent.SpanID(), rootSpanID)
		}
	}

	// Verify root span has key fingerprint attribute.
	found := false
	for _, attr := range rootSpan.Attributes {
		if string(attr.Key) == "ssh.key.fingerprint" {
			found = true
			break
		}
	}
	if !found {
		t.Error("root span missing ssh.key.fingerprint attribute")
	}

	// Verify device-lookup span has device attributes.
	if lookupSpan, ok := spanNames["device-lookup"]; ok {
		hasDeviceID := false
		for _, attr := range lookupSpan.Attributes {
			if string(attr.Key) == "device.id" {
				hasDeviceID = true
				break
			}
		}
		if !hasDeviceID {
			t.Error("device-lookup span missing device.id attribute")
		}
	}

	// Verify mtls-connect span has device IP attribute.
	if connectSpan, ok := spanNames["mtls-connect"]; ok {
		hasIP := false
		for _, attr := range connectSpan.Attributes {
			if string(attr.Key) == "device.ip" {
				hasIP = true
				break
			}
		}
		if !hasIP {
			t.Error("mtls-connect span missing device.ip attribute")
		}
	}
}

// TestIntegrationOTELNoopWhenDisabled verifies that when no tracer is
// configured (nil), the backend works normally with zero OTEL overhead.
func TestIntegrationOTELNoopWhenDisabled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, signer, grpcKey := newTestECDSAKey(t, "noop-key")
	phone := &testPhoneServer{
		keys:   []*nixkeyv1.SSHKey{grpcKey},
		signer: signer,
	}

	// Create backend with nil Tracer (should use no-op).
	backend, _ := setupTestBackend(t, phone)

	keys, err := backend.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}

	pub, err := ssh.ParsePublicKey(grpcKey.GetPublicKeyBlob())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	sig, err := backend.Sign(pub, []byte("noop test"), 0)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := pub.Verify([]byte("noop test"), sig); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

// TestIntegrationOTELSignErrorSpans verifies that error spans are recorded
// when a sign request fails.
func TestIntegrationOTELSignErrorSpans(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(trace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("nix-key-test")

	_, _, grpcKey := newTestECDSAKey(t, "error-key")
	phone := &testPhoneServer{
		keys:     []*nixkeyv1.SSHKey{grpcKey},
		denySign: true,
	}

	addr, cleanup := startTestPhoneServer(t, phone)
	t.Cleanup(cleanup)

	host, portStr, _ := net.SplitHostPort(addr)
	port := 0
	fmt.Sscanf(portStr, "%d", &port)

	registry := daemon.NewRegistry()
	registry.Add(daemon.Device{
		ID:              "error-device",
		Name:            "Error Phone",
		TailscaleIP:     host,
		ListenPort:      port,
		CertFingerprint: "error-fp",
		Source:          daemon.SourceRuntimePaired,
	})

	backend := agent.NewGRPCBackend(agent.GRPCBackendConfig{
		Registry:          registry,
		Dialer:            &otelTestDialer{},
		AllowKeyListing:   true,
		ConnectionTimeout: 5 * time.Second,
		SignTimeout:       5 * time.Second,
		Tracer:            tracer,
	})

	// List to populate cache.
	backend.List()
	exporter.Reset()

	pub, err := ssh.ParsePublicKey(grpcKey.GetPublicKeyBlob())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	_, err = backend.Sign(pub, []byte("error test"), 0)
	if err == nil {
		t.Fatal("expected error from denied sign")
	}

	spans := exporter.GetSpans()
	spanNames := make(map[string]tracetest.SpanStub)
	for _, s := range spans {
		spanNames[s.Name] = s
	}

	// Root span should exist and have error status.
	rootSpan, ok := spanNames["ssh-sign-request"]
	if !ok {
		t.Fatal("missing ssh-sign-request span on error path")
	}

	if rootSpan.Status.Code != otelcodes.Error {
		t.Errorf("root span should have error status, got %v", rootSpan.Status.Code)
	}

	// Should have recorded an error event.
	hasError := false
	for _, event := range rootSpan.Events {
		if event.Name == "exception" {
			hasError = true
			break
		}
	}
	if !hasError {
		t.Error("root span should have recorded an error event")
	}
}

func spanNameList(spans []tracetest.SpanStub) []string {
	names := make([]string, len(spans))
	for i, s := range spans {
		names[i] = s.Name
	}
	return names
}
