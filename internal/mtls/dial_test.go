package mtls_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	nixkeyv1 "github.com/phaedrus-raznikov/nix-key/gen/nixkey/v1"
	"github.com/phaedrus-raznikov/nix-key/internal/mtls"
	"google.golang.org/grpc"
)

// setupCertFiles generates certs and writes them to temp dir, returning paths.
func setupCertFiles(t *testing.T, name string) (certPath, keyPath string, certPEM []byte) {
	t.Helper()
	dir := t.TempDir()

	certPEM, keyPEM, err := mtls.GenerateCert(mtls.CertOptions{
		KeyType:    mtls.KeyTypeEd25519,
		CommonName: name,
	})
	if err != nil {
		t.Fatalf("generating %s cert: %v", name, err)
	}

	certPath = filepath.Join(dir, name+".crt")
	keyPath = filepath.Join(dir, name+".key")

	if err := os.WriteFile(certPath, certPEM, 0600); err != nil {
		t.Fatalf("writing cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		t.Fatalf("writing key: %v", err)
	}

	return certPath, keyPath, certPEM
}

func TestListenMTLS_And_DialMTLS(t *testing.T) {
	// Generate server and client certs
	serverCertPath, serverKeyPath, serverCertPEM := setupCertFiles(t, "server")
	clientCertPath, clientKeyPath, clientCertPEM := setupCertFiles(t, "client")

	serverFP, err := mtls.CertFingerprint(serverCertPEM)
	if err != nil {
		t.Fatalf("server fingerprint: %v", err)
	}
	clientFP, err := mtls.CertFingerprint(clientCertPEM)
	if err != nil {
		t.Fatalf("client fingerprint: %v", err)
	}

	// Start mTLS listener
	lis, err := mtls.ListenMTLS("127.0.0.1:0", serverCertPath, serverKeyPath, clientFP, "")
	if err != nil {
		t.Fatalf("ListenMTLS: %v", err)
	}
	defer lis.Close()

	// Dial with mTLS client
	conn, err := mtls.DialMTLS(lis.Addr().String(), clientCertPath, clientKeyPath, serverFP, "")
	if err != nil {
		t.Fatalf("DialMTLS: %v", err)
	}
	defer func() { _ = conn.Close() }()
}

func TestListenMTLS_And_DialMTLS_WithAge(t *testing.T) {
	dir := t.TempDir()

	// Generate age identity
	identityPath := filepath.Join(dir, "age.key")
	if err := mtls.GenerateIdentity(identityPath); err != nil {
		t.Fatalf("generating age identity: %v", err)
	}

	// Generate server cert (plaintext key)
	serverCertPEM, serverKeyPEM, err := mtls.GenerateCert(mtls.CertOptions{
		KeyType:    mtls.KeyTypeEd25519,
		CommonName: "server",
	})
	if err != nil {
		t.Fatalf("generating server cert: %v", err)
	}
	serverCertPath := filepath.Join(dir, "server.crt")
	serverKeyPath := filepath.Join(dir, "server.key")
	if err := os.WriteFile(serverCertPath, serverCertPEM, 0600); err != nil {
		t.Fatalf("writing server cert: %v", err)
	}
	if err := os.WriteFile(serverKeyPath, serverKeyPEM, 0600); err != nil {
		t.Fatalf("writing server key: %v", err)
	}
	// Encrypt server key with age
	if err := mtls.EncryptFile(serverKeyPath, identityPath); err != nil {
		t.Fatalf("encrypting server key: %v", err)
	}
	serverKeyAgePath := serverKeyPath + ".age"

	// Generate client cert (plaintext key)
	clientCertPEM, clientKeyPEM, err := mtls.GenerateCert(mtls.CertOptions{
		KeyType:    mtls.KeyTypeEd25519,
		CommonName: "client",
	})
	if err != nil {
		t.Fatalf("generating client cert: %v", err)
	}
	clientCertPath := filepath.Join(dir, "client.crt")
	clientKeyPath := filepath.Join(dir, "client.key")
	if err := os.WriteFile(clientCertPath, clientCertPEM, 0600); err != nil {
		t.Fatalf("writing client cert: %v", err)
	}
	if err := os.WriteFile(clientKeyPath, clientKeyPEM, 0600); err != nil {
		t.Fatalf("writing client key: %v", err)
	}
	// Encrypt client key with age
	if err := mtls.EncryptFile(clientKeyPath, identityPath); err != nil {
		t.Fatalf("encrypting client key: %v", err)
	}
	clientKeyAgePath := clientKeyPath + ".age"

	serverFP, _ := mtls.CertFingerprint(serverCertPEM)
	clientFP, _ := mtls.CertFingerprint(clientCertPEM)

	// Start mTLS listener with age-encrypted server key
	lis, err := mtls.ListenMTLS("127.0.0.1:0", serverCertPath, serverKeyAgePath, clientFP, identityPath)
	if err != nil {
		t.Fatalf("ListenMTLS with age: %v", err)
	}
	defer lis.Close()

	// Dial with mTLS client using age-encrypted client key
	conn, err := mtls.DialMTLS(lis.Addr().String(), clientCertPath, clientKeyAgePath, serverFP, identityPath)
	if err != nil {
		t.Fatalf("DialMTLS with age: %v", err)
	}
	defer func() { _ = conn.Close() }()
}

func TestDialMTLS_WrongFingerprint(t *testing.T) {
	serverCertPath, serverKeyPath, _ := setupCertFiles(t, "server")
	clientCertPath, clientKeyPath, clientCertPEM := setupCertFiles(t, "client")

	clientFP, _ := mtls.CertFingerprint(clientCertPEM)
	wrongFP := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	lis, err := mtls.ListenMTLS("127.0.0.1:0", serverCertPath, serverKeyPath, clientFP, "")
	if err != nil {
		t.Fatalf("ListenMTLS: %v", err)
	}
	defer lis.Close()

	// Client uses wrong fingerprint for server — should fail on first RPC
	conn, err := mtls.DialMTLS(lis.Addr().String(), clientCertPath, clientKeyPath, wrongFP, "")
	if err != nil {
		// grpc.NewClient may not fail immediately; that's OK
		return
	}
	defer func() { _ = conn.Close() }()

	// The handshake failure should manifest when making an actual RPC
	client := nixkeyv1.NewNixKeyAgentClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err = client.Ping(ctx, &nixkeyv1.PingRequest{})
	if err == nil {
		t.Fatal("expected RPC to fail with wrong server fingerprint")
	}
}

func TestListenMTLS_BadCertPath(t *testing.T) {
	_, err := mtls.ListenMTLS("127.0.0.1:0", "/nonexistent/cert.pem", "/nonexistent/key.pem", "abc", "")
	if err == nil {
		t.Fatal("expected error for nonexistent cert paths")
	}
}

// TestIntegrationMTLSGRPC establishes an mTLS connection between two goroutines
// and exchanges gRPC messages (Ping, ListKeys).
func TestIntegrationMTLSGRPC(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Generate server and client certs
	serverCertPath, serverKeyPath, serverCertPEM := setupCertFiles(t, "server")
	clientCertPath, clientKeyPath, clientCertPEM := setupCertFiles(t, "client")

	serverFP, _ := mtls.CertFingerprint(serverCertPEM)
	clientFP, _ := mtls.CertFingerprint(clientCertPEM)

	// Start mTLS listener
	lis, err := mtls.ListenMTLS("127.0.0.1:0", serverCertPath, serverKeyPath, clientFP, "")
	if err != nil {
		t.Fatalf("ListenMTLS: %v", err)
	}

	// Set up gRPC server with a mock phone server
	gs := grpc.NewServer()
	mockServer := &mockNixKeyAgent{}
	nixkeyv1.RegisterNixKeyAgentServer(gs, mockServer)

	errCh := make(chan error, 1)
	go func() {
		errCh <- gs.Serve(lis)
	}()
	defer gs.GracefulStop()

	// Dial with mTLS client
	conn, err := mtls.DialMTLS(lis.Addr().String(), clientCertPath, clientKeyPath, serverFP, "")
	if err != nil {
		t.Fatalf("DialMTLS: %v", err)
	}
	defer func() { _ = conn.Close() }()

	client := nixkeyv1.NewNixKeyAgentClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test Ping RPC
	pingResp, err := client.Ping(ctx, &nixkeyv1.PingRequest{})
	if err != nil {
		t.Fatalf("Ping RPC failed: %v", err)
	}
	if pingResp.TimestampMs <= 0 {
		t.Errorf("expected positive timestamp, got %d", pingResp.TimestampMs)
	}

	// Test ListKeys RPC
	listResp, err := client.ListKeys(ctx, &nixkeyv1.ListKeysRequest{})
	if err != nil {
		t.Fatalf("ListKeys RPC failed: %v", err)
	}
	if len(listResp.Keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(listResp.Keys))
	}
	if listResp.Keys[0].DisplayName != "test-key" {
		t.Errorf("expected key name 'test-key', got %q", listResp.Keys[0].DisplayName)
	}

	// Test Sign RPC
	signResp, err := client.Sign(ctx, &nixkeyv1.SignRequest{
		KeyFingerprint: "SHA256:test",
		Data:           []byte("test data to sign"),
		Flags:          0,
	})
	if err != nil {
		t.Fatalf("Sign RPC failed: %v", err)
	}
	if len(signResp.Signature) == 0 {
		t.Error("expected non-empty signature")
	}
}

// mockNixKeyAgent is a minimal gRPC server for testing.
type mockNixKeyAgent struct {
	nixkeyv1.UnimplementedNixKeyAgentServer
}

func (m *mockNixKeyAgent) Ping(_ context.Context, _ *nixkeyv1.PingRequest) (*nixkeyv1.PingResponse, error) {
	return &nixkeyv1.PingResponse{TimestampMs: time.Now().UnixMilli()}, nil
}

func (m *mockNixKeyAgent) ListKeys(_ context.Context, _ *nixkeyv1.ListKeysRequest) (*nixkeyv1.ListKeysResponse, error) {
	return &nixkeyv1.ListKeysResponse{
		Keys: []*nixkeyv1.SSHKey{
			{
				PublicKeyBlob: []byte("fake-blob"),
				KeyType:       "ed25519",
				DisplayName:   "test-key",
				Fingerprint:   "SHA256:test",
			},
		},
	}, nil
}

func (m *mockNixKeyAgent) Sign(_ context.Context, req *nixkeyv1.SignRequest) (*nixkeyv1.SignResponse, error) {
	return &nixkeyv1.SignResponse{Signature: []byte("fake-signature-for-" + req.KeyFingerprint)}, nil
}
