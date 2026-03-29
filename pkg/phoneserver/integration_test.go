package phoneserver_test

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	nixkeyv1 "github.com/phaedrus-raznikov/nix-key/gen/nixkey/v1"
	"github.com/phaedrus-raznikov/nix-key/pkg/phoneserver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// TestIntegrationGRPCRoundTrip starts a gRPC server in a goroutine, connects
// a gRPC client, exercises all three RPCs, and verifies protobuf field fidelity.
func TestIntegrationGRPCRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	keys := []*phoneserver.Key{
		{
			PublicKeyBlob: []byte("ssh-ed25519-blob-data-here"),
			KeyType:       "ssh-ed25519",
			DisplayName:   "my-ed25519-key",
			Fingerprint:   "SHA256:roundtrip1",
		},
		{
			PublicKeyBlob: []byte("ecdsa-sha2-nistp256-blob-data"),
			KeyType:       "ecdsa-sha2-nistp256",
			DisplayName:   "my-ecdsa-key",
			Fingerprint:   "SHA256:roundtrip2",
		},
	}

	ks := &mockKeyStore{keys: keys}
	srv := phoneserver.NewServer(ks, &autoApproveConfirmer{})

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
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
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := nixkeyv1.NewNixKeyAgentClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// --- Ping RPC ---
	t.Run("Ping", func(t *testing.T) {
		before := time.Now().UnixMilli()
		resp, err := client.Ping(ctx, &nixkeyv1.PingRequest{})
		if err != nil {
			t.Fatalf("Ping: %v", err)
		}
		after := time.Now().UnixMilli()

		if resp.TimestampMs < before || resp.TimestampMs > after {
			t.Errorf("timestamp %d not in [%d, %d]", resp.TimestampMs, before, after)
		}
	})

	// --- ListKeys RPC: verify all protobuf fields round-trip ---
	t.Run("ListKeys", func(t *testing.T) {
		resp, err := client.ListKeys(ctx, &nixkeyv1.ListKeysRequest{})
		if err != nil {
			t.Fatalf("ListKeys: %v", err)
		}

		if got := len(resp.Keys); got != len(keys) {
			t.Fatalf("expected %d keys, got %d", len(keys), got)
		}

		for i, want := range keys {
			got := resp.Keys[i]
			if !bytes.Equal(got.PublicKeyBlob, want.PublicKeyBlob) {
				t.Errorf("key[%d] PublicKeyBlob = %q, want %q", i, got.PublicKeyBlob, want.PublicKeyBlob)
			}
			if got.KeyType != want.KeyType {
				t.Errorf("key[%d] KeyType = %q, want %q", i, got.KeyType, want.KeyType)
			}
			if got.DisplayName != want.DisplayName {
				t.Errorf("key[%d] DisplayName = %q, want %q", i, got.DisplayName, want.DisplayName)
			}
			if got.Fingerprint != want.Fingerprint {
				t.Errorf("key[%d] Fingerprint = %q, want %q", i, got.Fingerprint, want.Fingerprint)
			}
		}
	})

	// --- Sign RPC: verify signature round-trip ---
	t.Run("Sign", func(t *testing.T) {
		data := []byte("integration test payload")
		resp, err := client.Sign(ctx, &nixkeyv1.SignRequest{
			KeyFingerprint: "SHA256:roundtrip1",
			Data:           data,
			Flags:          42,
		})
		if err != nil {
			t.Fatalf("Sign: %v", err)
		}

		if len(resp.Signature) == 0 {
			t.Fatal("expected non-empty signature")
		}
		// Mock returns "signed:" + first 8 bytes of data
		wantPrefix := []byte("signed:")
		if !bytes.HasPrefix(resp.Signature, wantPrefix) {
			t.Errorf("signature = %q, want prefix %q", resp.Signature, wantPrefix)
		}
	})
}

// TestIntegrationSignUnknownKey verifies that signing with a fingerprint that
// doesn't match any key in the store returns a gRPC error.
func TestIntegrationSignUnknownKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	keys := []*phoneserver.Key{
		{
			PublicKeyBlob: []byte("blob"),
			KeyType:       "ssh-ed25519",
			DisplayName:   "known-key",
			Fingerprint:   "SHA256:known",
		},
	}

	client, cleanup := startIntegrationServer(t, &mockKeyStore{keys: keys}, &autoApproveConfirmer{})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.Sign(ctx, &nixkeyv1.SignRequest{
		KeyFingerprint: "SHA256:does-not-exist",
		Data:           []byte("some data"),
		Flags:          0,
	})
	if err == nil {
		t.Fatal("expected error for unknown key fingerprint")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %T: %v", err, err)
	}
	if st.Code() != codes.Internal {
		t.Errorf("expected codes.Internal, got %v", st.Code())
	}
	if st.Message() == "" {
		t.Error("expected non-empty error message")
	}
}

// TestIntegrationSignConfirmerDenies verifies that when the Confirmer denies a
// sign request, the client receives a PermissionDenied gRPC error.
func TestIntegrationSignConfirmerDenies(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	keys := []*phoneserver.Key{
		{
			PublicKeyBlob: []byte("blob"),
			KeyType:       "ssh-ed25519",
			DisplayName:   "denied-key",
			Fingerprint:   "SHA256:deny-me",
		},
	}

	client, cleanup := startIntegrationServer(t, &mockKeyStore{keys: keys}, &denyConfirmer{})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.Sign(ctx, &nixkeyv1.SignRequest{
		KeyFingerprint: "SHA256:deny-me",
		Data:           []byte("data to sign"),
		Flags:          0,
	})
	if err == nil {
		t.Fatal("expected error when confirmer denies")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %T: %v", err, err)
	}
	if st.Code() != codes.PermissionDenied {
		t.Errorf("expected codes.PermissionDenied, got %v", st.Code())
	}
}

// startIntegrationServer is a helper that starts a gRPC server and returns
// a connected client plus a cleanup function.
func startIntegrationServer(t *testing.T, ks phoneserver.KeyStore, conf phoneserver.Confirmer) (nixkeyv1.NixKeyAgentClient, func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := phoneserver.NewServer(ks, conf)
	gs := grpc.NewServer()
	nixkeyv1.RegisterNixKeyAgentServer(gs, srv)
	go func() { _ = gs.Serve(lis) }()

	conn, err := grpc.NewClient(
		fmt.Sprintf("127.0.0.1:%d", lis.Addr().(*net.TCPAddr).Port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
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
