package phoneserver

import (
	"context"
	"fmt"
	"net"
	"sync"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"google.golang.org/grpc"
)

// PhoneServer is the gomobile-exported type that wraps the gRPC server.
// Android creates an instance via NewPhoneServer, then calls StartOnAddress.
type PhoneServer struct {
	server       *Server
	mu           sync.Mutex
	lis          net.Listener
	otelEndpoint string
	tp           *sdktrace.TracerProvider
}

// NewPhoneServer creates a new PhoneServer backed by the given KeyStore and Confirmer.
// This is the main entry point called from Android (GoPhoneServer.kt).
func NewPhoneServer(ks KeyStore, conf Confirmer) *PhoneServer {
	return &PhoneServer{
		server: NewServer(ks, conf),
	}
}

// SetOTELEndpoint configures the OTLP exporter endpoint (e.g., "localhost:4317").
// Must be called before StartOnAddress. Pass empty string to disable tracing.
func (ps *PhoneServer) SetOTELEndpoint(endpoint string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.otelEndpoint = endpoint
}

// StartOnAddress starts the gRPC server listening on the given address (e.g., "0.0.0.0:50051").
// This blocks until the server is stopped. Call from a background goroutine / thread.
func (ps *PhoneServer) StartOnAddress(addr string) error {
	ps.mu.Lock()
	if ps.lis != nil {
		ps.mu.Unlock()
		return fmt.Errorf("server already started")
	}

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		ps.mu.Unlock()
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	ps.lis = lis

	var serverOpts []grpc.ServerOption
	if ps.otelEndpoint != "" {
		tp, initErr := ps.initTracing(ps.otelEndpoint)
		if initErr != nil {
			ps.lis = nil
			lis.Close()
			ps.mu.Unlock()
			return fmt.Errorf("init tracing: %w", initErr)
		}
		ps.tp = tp
		ps.server = NewServerWithTracing(ps.server.ks, ps.server.conf, tp)
		serverOpts = append(serverOpts, grpc.StatsHandler(otelgrpc.NewServerHandler(
			otelgrpc.WithTracerProvider(tp),
		)))
	}
	ps.mu.Unlock()

	return ps.server.StartWithOpts(lis, serverOpts...)
}

// Port returns the port the server is listening on, or 0 if not started.
func (ps *PhoneServer) Port() int {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.lis == nil {
		return 0
	}
	return ps.lis.Addr().(*net.TCPAddr).Port
}

// Stop gracefully stops the gRPC server and closes the listener.
func (ps *PhoneServer) Stop() {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.server.Stop()
	if ps.tp != nil {
		_ = ps.tp.Shutdown(context.Background())
		ps.tp = nil
	}
	ps.lis = nil
}

// initTracing creates an OTLP gRPC tracer provider for the given endpoint.
func (ps *PhoneServer) initTracing(endpoint string) (*sdktrace.TracerProvider, error) {
	ctx := context.Background()
	exp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("creating OTLP exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName("nix-key-phone")),
	)
	if err != nil {
		return nil, fmt.Errorf("creating resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	return tp, nil
}
