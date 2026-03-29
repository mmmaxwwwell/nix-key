package phoneserver_test

import (
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

// mockKeyStore is an in-memory KeyStore for testing.
type mockKeyStore struct {
	keys []*phoneserver.Key
}

func (m *mockKeyStore) ListKeys() (*phoneserver.KeyList, error) {
	kl := phoneserver.NewKeyList()
	for _, k := range m.keys {
		kl.Add(k)
	}
	return kl, nil
}

func (m *mockKeyStore) Sign(fingerprint string, data []byte, _ int32) ([]byte, error) {
	for _, k := range m.keys {
		if k.Fingerprint == fingerprint {
			// Return a fake signature: "signed:" + first 8 bytes of data
			sig := append([]byte("signed:"), data[:min(8, len(data))]...)
			return sig, nil
		}
	}
	return nil, fmt.Errorf("key not found: %s", fingerprint)
}

// autoApproveConfirmer always approves.
type autoApproveConfirmer struct{}

func (a *autoApproveConfirmer) RequestConfirmation(_, _, _ string) (bool, error) {
	return true, nil
}

// denyConfirmer always denies.
type denyConfirmer struct{}

func (d *denyConfirmer) RequestConfirmation(_, _, _ string) (bool, error) {
	return false, nil
}

func testKeys() []*phoneserver.Key {
	return []*phoneserver.Key{
		{
			PublicKeyBlob: []byte("ed25519-pub-blob"),
			KeyType:       "ssh-ed25519",
			DisplayName:   "test-key-1",
			Fingerprint:   "SHA256:abc123",
		},
		{
			PublicKeyBlob: []byte("ecdsa-pub-blob"),
			KeyType:       "ecdsa-sha2-nistp256",
			DisplayName:   "test-key-2",
			Fingerprint:   "SHA256:def456",
		},
	}
}

func startTestServer(t *testing.T, ks phoneserver.KeyStore, conf phoneserver.Confirmer) (nixkeyv1.NixKeyAgentClient, func()) {
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
		lis.Addr().String(),
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

func TestListKeys(t *testing.T) {
	ks := &mockKeyStore{keys: testKeys()}
	client, cleanup := startTestServer(t, ks, &autoApproveConfirmer{})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.ListKeys(ctx, &nixkeyv1.ListKeysRequest{})
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}

	if len(resp.Keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(resp.Keys))
	}

	if resp.Keys[0].Fingerprint != "SHA256:abc123" {
		t.Errorf("key[0] fingerprint = %q, want %q", resp.Keys[0].Fingerprint, "SHA256:abc123")
	}
	if resp.Keys[1].KeyType != "ecdsa-sha2-nistp256" {
		t.Errorf("key[1] type = %q, want %q", resp.Keys[1].KeyType, "ecdsa-sha2-nistp256")
	}
}

func TestSign(t *testing.T) {
	ks := &mockKeyStore{keys: testKeys()}
	client, cleanup := startTestServer(t, ks, &autoApproveConfirmer{})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.Sign(ctx, &nixkeyv1.SignRequest{
		KeyFingerprint: "SHA256:abc123",
		Data:           []byte("test data to sign"),
		Flags:          0,
	})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	if len(resp.Signature) == 0 {
		t.Fatal("expected non-empty signature")
	}
}

func TestSignUnknownKey(t *testing.T) {
	ks := &mockKeyStore{keys: testKeys()}
	client, cleanup := startTestServer(t, ks, &autoApproveConfirmer{})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.Sign(ctx, &nixkeyv1.SignRequest{
		KeyFingerprint: "SHA256:nonexistent",
		Data:           []byte("data"),
		Flags:          0,
	})
	if err == nil {
		t.Fatal("expected error for unknown key")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.Internal {
		t.Errorf("expected Internal code, got %v", st.Code())
	}
}

func TestSignDenied(t *testing.T) {
	ks := &mockKeyStore{keys: testKeys()}
	client, cleanup := startTestServer(t, ks, &denyConfirmer{})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.Sign(ctx, &nixkeyv1.SignRequest{
		KeyFingerprint: "SHA256:abc123",
		Data:           []byte("data"),
		Flags:          0,
	})
	if err == nil {
		t.Fatal("expected error when user denies")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied code, got %v", st.Code())
	}
}

func TestPing(t *testing.T) {
	ks := &mockKeyStore{keys: nil}
	client, cleanup := startTestServer(t, ks, &autoApproveConfirmer{})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	before := time.Now().UnixMilli()
	resp, err := client.Ping(ctx, &nixkeyv1.PingRequest{})
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
	after := time.Now().UnixMilli()

	if resp.TimestampMs < before || resp.TimestampMs > after {
		t.Errorf("timestamp %d not in range [%d, %d]", resp.TimestampMs, before, after)
	}
}

func TestSignMissingFingerprint(t *testing.T) {
	ks := &mockKeyStore{keys: testKeys()}
	client, cleanup := startTestServer(t, ks, &autoApproveConfirmer{})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.Sign(ctx, &nixkeyv1.SignRequest{
		Data:  []byte("data"),
		Flags: 0,
	})
	if err == nil {
		t.Fatal("expected error for missing fingerprint")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument code, got %v", st.Code())
	}
}

func TestPhoneServerBridge(t *testing.T) {
	ks := &mockKeyStore{keys: testKeys()}
	ps := phoneserver.NewPhoneServer(ks, &autoApproveConfirmer{})

	// Start server in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- ps.StartOnAddress("127.0.0.1:0")
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	port := ps.Port()
	if port == 0 {
		t.Fatal("server did not start")
	}

	// Connect and test
	conn, err := grpc.NewClient(
		fmt.Sprintf("127.0.0.1:%d", port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := nixkeyv1.NewNixKeyAgentClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test Ping via bridge
	resp, err := client.Ping(ctx, &nixkeyv1.PingRequest{})
	if err != nil {
		t.Fatalf("Ping via bridge: %v", err)
	}
	if resp.TimestampMs == 0 {
		t.Error("expected non-zero timestamp")
	}

	// Test ListKeys via bridge
	keysResp, err := client.ListKeys(ctx, &nixkeyv1.ListKeysRequest{})
	if err != nil {
		t.Fatalf("ListKeys via bridge: %v", err)
	}
	if len(keysResp.Keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(keysResp.Keys))
	}

	ps.Stop()
}
