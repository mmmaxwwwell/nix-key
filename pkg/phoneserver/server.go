package phoneserver

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"time"

	nixkeyv1 "github.com/phaedrus-raznikov/nix-key/gen/nixkey/v1"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server is the gRPC server that implements the NixKeyAgent service.
// It delegates key operations to KeyStore and confirmation to Confirmer.
type Server struct {
	nixkeyv1.UnimplementedNixKeyAgentServer
	ks     KeyStore
	conf   Confirmer
	gs     *grpc.Server
	tracer trace.Tracer
}

// NewServer creates a new Server backed by the given KeyStore and Confirmer.
// Tracing is disabled (no-op) by default. Use NewServerWithTracing to enable.
func NewServer(ks KeyStore, conf Confirmer) *Server {
	return &Server{
		ks:     ks,
		conf:   conf,
		tracer: noop.NewTracerProvider().Tracer("nix-key-phone"),
	}
}

// NewServerWithTracing creates a new Server with OTEL tracing enabled.
// The provided TracerProvider is used to create spans for sign operations.
// The caller should also register an otelgrpc server handler on the gRPC
// server to extract traceparent from incoming metadata.
func NewServerWithTracing(ks KeyStore, conf Confirmer, tp trace.TracerProvider) *Server {
	return &Server{
		ks:     ks,
		conf:   conf,
		tracer: tp.Tracer("nix-key-phone"),
	}
}

// ListKeys implements the NixKeyAgent.ListKeys RPC.
func (s *Server) ListKeys(_ context.Context, _ *nixkeyv1.ListKeysRequest) (*nixkeyv1.ListKeysResponse, error) {
	kl, err := s.ks.ListKeys()
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to list keys")
	}

	resp := &nixkeyv1.ListKeysResponse{}
	for i := 0; i < kl.Len(); i++ {
		k := kl.Get(i)
		if k == nil {
			continue
		}
		resp.Keys = append(resp.Keys, &nixkeyv1.SSHKey{
			PublicKeyBlob: k.PublicKeyBlob,
			KeyType:       k.KeyType,
			DisplayName:   k.DisplayName,
			Fingerprint:   k.Fingerprint,
		})
	}
	return resp, nil
}

// Sign implements the NixKeyAgent.Sign RPC.
func (s *Server) Sign(ctx context.Context, req *nixkeyv1.SignRequest) (*nixkeyv1.SignResponse, error) {
	ctx, handleSpan := s.tracer.Start(ctx, "handle-sign-request",
		trace.WithAttributes(attribute.String("key.fingerprint", req.GetKeyFingerprint())),
	)
	defer handleSpan.End()

	if req.GetKeyFingerprint() == "" {
		return nil, status.Error(codes.InvalidArgument, "key_fingerprint is required")
	}
	if len(req.GetData()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "data is required")
	}

	// Compute truncated data hash for display
	hash := sha256.Sum256(req.GetData())
	dataHash := fmt.Sprintf("%x", hash[:8])

	// Look up key name from the key store
	keyName := req.GetKeyFingerprint()
	kl, err := s.ks.ListKeys()
	if err == nil {
		for i := 0; i < kl.Len(); i++ {
			k := kl.Get(i)
			if k == nil {
				continue
			}
			if k.Fingerprint == req.GetKeyFingerprint() {
				keyName = k.DisplayName
				break
			}
		}
	}

	// Show prompt and wait for user confirmation.
	_, promptSpan := s.tracer.Start(ctx, "show-prompt",
		trace.WithAttributes(
			attribute.String("key.name", keyName),
			attribute.String("data.hash", dataHash),
		),
	)
	approved, err := s.conf.RequestConfirmation("host", keyName, dataHash)
	promptSpan.End()

	if err != nil {
		return nil, status.Error(codes.Internal, "confirmation failed")
	}

	// Record user response.
	_, responseSpan := s.tracer.Start(ctx, "user-response",
		trace.WithAttributes(attribute.Bool("user.approved", approved)),
	)
	responseSpan.End()

	if !approved {
		return nil, status.Error(codes.PermissionDenied, "user denied sign request")
	}

	// Perform keystore signing.
	_, signSpan := s.tracer.Start(ctx, "keystore-sign",
		trace.WithAttributes(attribute.String("key.fingerprint", req.GetKeyFingerprint())),
	)
	sig, err := s.ks.Sign(req.GetKeyFingerprint(), req.GetData(), int32(req.GetFlags()))
	if err != nil {
		signSpan.RecordError(err)
		signSpan.End()
		return nil, status.Error(codes.Internal, "signing failed")
	}
	signSpan.End()

	return &nixkeyv1.SignResponse{Signature: sig}, nil
}

// Ping implements the NixKeyAgent.Ping RPC.
func (s *Server) Ping(_ context.Context, _ *nixkeyv1.PingRequest) (*nixkeyv1.PingResponse, error) {
	return &nixkeyv1.PingResponse{
		TimestampMs: time.Now().UnixMilli(),
	}, nil
}

// Start starts the gRPC server on the given listener.
func (s *Server) Start(lis net.Listener) error {
	return s.StartWithOpts(lis)
}

// StartWithOpts starts the gRPC server with additional server options (e.g., otelgrpc handler).
func (s *Server) StartWithOpts(lis net.Listener, opts ...grpc.ServerOption) error {
	s.gs = grpc.NewServer(opts...)
	nixkeyv1.RegisterNixKeyAgentServer(s.gs, s)
	return s.gs.Serve(lis)
}

// Stop gracefully stops the gRPC server.
func (s *Server) Stop() {
	if s.gs != nil {
		s.gs.GracefulStop()
	}
}
