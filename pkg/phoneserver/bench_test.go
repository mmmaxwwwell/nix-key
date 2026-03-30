package phoneserver_test

import (
	"context"
	"net"
	"testing"
	"time"

	nixkeyv1 "github.com/phaedrus-raznikov/nix-key/gen/nixkey/v1"
	"github.com/phaedrus-raznikov/nix-key/pkg/phoneserver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// BenchmarkGRPCRoundTrip measures the latency of a single gRPC Sign RPC
// through an in-process phone server with auto-approve.
func BenchmarkGRPCRoundTrip(b *testing.B) {
	keys := []*phoneserver.Key{
		{
			PublicKeyBlob: []byte("ed25519-pub-blob"),
			KeyType:       "ssh-ed25519",
			DisplayName:   "bench-key",
			Fingerprint:   "SHA256:bench123",
		},
	}

	ks := &mockKeyStore{keys: keys}
	srv := phoneserver.NewServer(ks, &autoApproveConfirmer{})

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("listen: %v", err)
	}

	gs := grpc.NewServer()
	nixkeyv1.RegisterNixKeyAgentServer(gs, srv)
	go func() { _ = gs.Serve(lis) }()
	defer gs.GracefulStop()

	conn, err := grpc.NewClient(
		lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		b.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	client := nixkeyv1.NewNixKeyAgentClient(conn)

	// Warm up the connection.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := client.Ping(ctx, &nixkeyv1.PingRequest{}); err != nil {
		b.Fatalf("warmup ping: %v", err)
	}

	req := &nixkeyv1.SignRequest{
		KeyFingerprint: "SHA256:bench123",
		Data:           []byte("benchmark data to sign"),
		Flags:          0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.Sign(context.Background(), req)
		if err != nil {
			b.Fatalf("sign: %v", err)
		}
	}
}
