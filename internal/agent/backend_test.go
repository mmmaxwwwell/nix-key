package agent_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sshagent "golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/phaedrus-raznikov/nix-key/internal/agent"
	"github.com/phaedrus-raznikov/nix-key/internal/daemon"

	nixkeyv1 "github.com/phaedrus-raznikov/nix-key/gen/nixkey/v1"
)

// testPhoneServer implements nixkeyv1.NixKeyAgentServer for testing.
type testPhoneServer struct {
	nixkeyv1.UnimplementedNixKeyAgentServer
	keys   []*nixkeyv1.SSHKey
	signer ssh.Signer // for signing data

	signDelay  time.Duration // simulated delay before signing
	denySign   bool          // if true, return error on Sign
	signCount  atomic.Int64  // tracks number of sign calls
}

func (s *testPhoneServer) ListKeys(_ context.Context, _ *nixkeyv1.ListKeysRequest) (*nixkeyv1.ListKeysResponse, error) {
	return &nixkeyv1.ListKeysResponse{Keys: s.keys}, nil
}

func (s *testPhoneServer) Sign(_ context.Context, req *nixkeyv1.SignRequest) (*nixkeyv1.SignResponse, error) {
	s.signCount.Add(1)

	if s.signDelay > 0 {
		time.Sleep(s.signDelay)
	}
	if s.denySign {
		return nil, status.Error(codes.PermissionDenied, "user denied")
	}

	sig, err := s.signer.Sign(rand.Reader, req.GetData())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "sign: %v", err)
	}

	return &nixkeyv1.SignResponse{
		Signature: ssh.Marshal(sig),
	}, nil
}

func (s *testPhoneServer) Ping(_ context.Context, _ *nixkeyv1.PingRequest) (*nixkeyv1.PingResponse, error) {
	return &nixkeyv1.PingResponse{TimestampMs: time.Now().UnixMilli()}, nil
}

// startTestPhoneServer starts an in-process gRPC phone server and returns
// its address and cleanup function.
func startTestPhoneServer(t *testing.T, srv nixkeyv1.NixKeyAgentServer) (string, func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	gs := grpc.NewServer()
	nixkeyv1.RegisterNixKeyAgentServer(gs, srv)
	go gs.Serve(lis)

	return lis.Addr().String(), func() {
		gs.GracefulStop()
	}
}

// testDialer implements agent.Dialer using plain gRPC (no mTLS) for testing.
type testDialer struct {
	dialErr error // if set, all dials return this error
}

func (d *testDialer) DialDevice(ctx context.Context, dev daemon.Device) (nixkeyv1.NixKeyAgentClient, func(), error) {
	if d.dialErr != nil {
		return nil, nil, d.dialErr
	}

	addr := fmt.Sprintf("%s:%d", dev.TailscaleIP, dev.ListenPort)
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, nil, err
	}

	client := nixkeyv1.NewNixKeyAgentClient(conn)
	cleanup := func() { conn.Close() }
	return client, cleanup, nil
}

// newTestECDSAKey generates a test ECDSA-P256 key and returns the SSH public key
// info, signer, and gRPC SSHKey representation.
func newTestECDSAKey(t *testing.T, name string) (*sshagent.Key, ssh.Signer, *nixkeyv1.SSHKey) {
	t.Helper()
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(privKey)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	pub := signer.PublicKey()
	fp := ssh.FingerprintSHA256(pub)

	agentKey := &sshagent.Key{
		Format:  pub.Type(),
		Blob:    pub.Marshal(),
		Comment: name,
	}

	grpcKey := &nixkeyv1.SSHKey{
		PublicKeyBlob: pub.Marshal(),
		KeyType:       pub.Type(),
		DisplayName:   name,
		Fingerprint:   fp,
	}

	return agentKey, signer, grpcKey
}

// setupTestBackend creates a registry with one device, an in-process gRPC phone
// server, and a GRPCBackend wired together for testing.
func setupTestBackend(t *testing.T, phone *testPhoneServer, opts ...func(*agent.GRPCBackendConfig)) (*agent.GRPCBackend, *daemon.Registry) {
	t.Helper()

	addr, cleanup := startTestPhoneServer(t, phone)
	t.Cleanup(cleanup)

	// Parse host:port.
	host, portStr, _ := net.SplitHostPort(addr)
	port := 0
	fmt.Sscanf(portStr, "%d", &port)

	// Create a device in the registry.
	registry := daemon.NewRegistry()
	dev := daemon.Device{
		ID:              "test-device-1",
		Name:            "Test Phone",
		TailscaleIP:     host,
		ListenPort:      port,
		CertFingerprint: fmt.Sprintf("%x", sha256.Sum256([]byte("test-cert"))),
		Source:          daemon.SourceRuntimePaired,
	}
	registry.Add(dev)

	cfg := agent.GRPCBackendConfig{
		Registry:          registry,
		Dialer:            &testDialer{},
		AllowKeyListing:   true,
		ConnectionTimeout: 5 * time.Second,
		SignTimeout:       5 * time.Second,
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	backend := agent.NewGRPCBackend(cfg)
	return backend, registry
}

// --- Integration Tests ---

func TestIntegrationGRPCBackendListKeys(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, _, grpcKey1 := newTestECDSAKey(t, "key-1")
	_, _, grpcKey2 := newTestECDSAKey(t, "key-2")

	phone := &testPhoneServer{
		keys: []*nixkeyv1.SSHKey{grpcKey1, grpcKey2},
	}

	backend, _ := setupTestBackend(t, phone)

	keys, err := backend.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}

	// Verify key details match.
	names := map[string]bool{}
	for _, k := range keys {
		names[k.Comment] = true
	}
	if !names["key-1"] || !names["key-2"] {
		t.Errorf("expected key-1 and key-2 in results, got %v", names)
	}
}

func TestIntegrationGRPCBackendListKeysDisabled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, _, grpcKey1 := newTestECDSAKey(t, "key-1")
	phone := &testPhoneServer{
		keys: []*nixkeyv1.SSHKey{grpcKey1},
	}

	backend, _ := setupTestBackend(t, phone, func(cfg *agent.GRPCBackendConfig) {
		cfg.AllowKeyListing = false
	})

	keys, err := backend.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("expected 0 keys when listing disabled, got %d", len(keys))
	}
}

func TestIntegrationGRPCBackendSign(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, signer, grpcKey := newTestECDSAKey(t, "sign-key")
	phone := &testPhoneServer{
		keys:   []*nixkeyv1.SSHKey{grpcKey},
		signer: signer,
	}

	backend, _ := setupTestBackend(t, phone)

	// First list keys to populate cache.
	keys, err := backend.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}

	// Parse public key for signing.
	pub, err := ssh.ParsePublicKey(grpcKey.GetPublicKeyBlob())
	if err != nil {
		t.Fatalf("parse public key: %v", err)
	}

	data := []byte("data to sign")
	sig, err := backend.Sign(pub, data, 0)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if sig == nil {
		t.Fatal("signature is nil")
	}

	// Verify signature.
	if err := pub.Verify(data, sig); err != nil {
		t.Fatalf("verify signature: %v", err)
	}
}

func TestIntegrationGRPCBackendSignAutoRefreshCache(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, signer, grpcKey := newTestECDSAKey(t, "auto-refresh-key")
	phone := &testPhoneServer{
		keys:   []*nixkeyv1.SSHKey{grpcKey},
		signer: signer,
	}

	backend, _ := setupTestBackend(t, phone)

	// Sign without listing first — should auto-refresh cache.
	pub, err := ssh.ParsePublicKey(grpcKey.GetPublicKeyBlob())
	if err != nil {
		t.Fatalf("parse public key: %v", err)
	}

	data := []byte("auto refresh test")
	sig, err := backend.Sign(pub, data, 0)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := pub.Verify(data, sig); err != nil {
		t.Fatalf("verify signature: %v", err)
	}
}

func TestIntegrationGRPCBackendSignDenied(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, _, grpcKey := newTestECDSAKey(t, "deny-key")
	phone := &testPhoneServer{
		keys:     []*nixkeyv1.SSHKey{grpcKey},
		denySign: true,
	}

	backend, _ := setupTestBackend(t, phone)

	// List to populate cache.
	backend.List()

	pub, err := ssh.ParsePublicKey(grpcKey.GetPublicKeyBlob())
	if err != nil {
		t.Fatalf("parse public key: %v", err)
	}

	_, err = backend.Sign(pub, []byte("test"), 0)
	if err == nil {
		t.Fatal("expected error from denied sign, got nil")
	}
}

func TestIntegrationGRPCBackendSignTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, signer, grpcKey := newTestECDSAKey(t, "timeout-key")
	phone := &testPhoneServer{
		keys:      []*nixkeyv1.SSHKey{grpcKey},
		signer:    signer,
		signDelay: 10 * time.Second, // much longer than signTimeout
	}

	backend, _ := setupTestBackend(t, phone, func(cfg *agent.GRPCBackendConfig) {
		cfg.SignTimeout = 200 * time.Millisecond
	})

	// List to populate cache.
	backend.List()

	pub, err := ssh.ParsePublicKey(grpcKey.GetPublicKeyBlob())
	if err != nil {
		t.Fatalf("parse public key: %v", err)
	}

	_, err = backend.Sign(pub, []byte("test"), 0)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestIntegrationGRPCBackendSignUnknownKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, _, grpcKey := newTestECDSAKey(t, "known-key")
	phone := &testPhoneServer{
		keys: []*nixkeyv1.SSHKey{grpcKey},
	}

	backend, _ := setupTestBackend(t, phone)

	// Generate a completely different key not on the phone.
	unknownKey, _ := newTestKey(t)
	pub, err := ssh.ParsePublicKey(unknownKey.Blob)
	if err != nil {
		t.Fatalf("parse public key: %v", err)
	}

	_, err = backend.Sign(pub, []byte("test"), 0)
	if err == nil {
		t.Fatal("expected error for unknown key, got nil")
	}
}

func TestIntegrationGRPCBackendDialFailureLogging(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// FR-E05: mTLS failure should be logged with details.
	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)

	registry := daemon.NewRegistry()
	dev := daemon.Device{
		ID:              "unreachable-device",
		Name:            "Unreachable Phone",
		TailscaleIP:     "127.0.0.1",
		ListenPort:      1, // unlikely to have a server
		CertFingerprint: "deadbeef",
		Source:          daemon.SourceRuntimePaired,
	}
	registry.Add(dev)

	// Use a dialer that returns an error.
	failDialer := &testDialer{dialErr: fmt.Errorf("mTLS handshake: certificate expired")}

	backend := agent.NewGRPCBackend(agent.GRPCBackendConfig{
		Registry:          registry,
		Dialer:            failDialer,
		AllowKeyListing:   true,
		ConnectionTimeout: 1 * time.Second,
		SignTimeout:       1 * time.Second,
		Logger:            logger,
	})

	// Populate cache manually won't work since list fails.
	// Instead, try listing — it should log the dial failure.
	keys, err := backend.List()
	if err != nil {
		t.Fatalf("list should not error (skip unreachable), got: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("expected 0 keys from unreachable device, got %d", len(keys))
	}

	// Verify log contains device details (FR-E05).
	logStr := logBuf.String()
	if !containsStr(logStr, "Unreachable Phone") {
		t.Errorf("log should contain device name, got: %s", logStr)
	}
	if !containsStr(logStr, "certificate expired") {
		t.Errorf("log should contain error details, got: %s", logStr)
	}
	if !containsStr(logStr, "deadbeef") {
		t.Errorf("log should contain cert fingerprint, got: %s", logStr)
	}
}

// TestIntegrationGRPCBackendConcurrentSignRequests tests that multiple SSH
// clients can issue sign requests concurrently (T-HI-08).
func TestIntegrationGRPCBackendConcurrentSignRequests(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, signer, grpcKey := newTestECDSAKey(t, "concurrent-key")
	phone := &testPhoneServer{
		keys:   []*nixkeyv1.SSHKey{grpcKey},
		signer: signer,
	}

	backend, _ := setupTestBackend(t, phone)

	// Populate cache.
	backend.List()

	pub, err := ssh.ParsePublicKey(grpcKey.GetPublicKeyBlob())
	if err != nil {
		t.Fatalf("parse public key: %v", err)
	}

	// Launch 10 concurrent sign requests.
	const numClients = 10
	var wg sync.WaitGroup
	errCh := make(chan error, numClients)

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			data := []byte(fmt.Sprintf("concurrent data %d", idx))
			sig, err := backend.Sign(pub, data, 0)
			if err != nil {
				errCh <- fmt.Errorf("client %d sign: %w", idx, err)
				return
			}
			if err := pub.Verify(data, sig); err != nil {
				errCh <- fmt.Errorf("client %d verify: %w", idx, err)
				return
			}
			errCh <- nil
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Error(err)
		}
	}

	// Verify all sign requests reached the phone.
	if got := phone.signCount.Load(); got != numClients {
		t.Errorf("expected %d sign calls, got %d", numClients, got)
	}
}

// TestIntegrationGRPCBackendMultipleDevices tests listing keys from
// multiple phone devices.
func TestIntegrationGRPCBackendMultipleDevices(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Phone 1 with 1 key.
	_, _, grpcKey1 := newTestECDSAKey(t, "phone1-key")
	phone1 := &testPhoneServer{keys: []*nixkeyv1.SSHKey{grpcKey1}}
	addr1, cleanup1 := startTestPhoneServer(t, phone1)
	t.Cleanup(cleanup1)

	// Phone 2 with 2 keys.
	_, _, grpcKey2 := newTestECDSAKey(t, "phone2-key-a")
	_, _, grpcKey3 := newTestECDSAKey(t, "phone2-key-b")
	phone2 := &testPhoneServer{keys: []*nixkeyv1.SSHKey{grpcKey2, grpcKey3}}
	addr2, cleanup2 := startTestPhoneServer(t, phone2)
	t.Cleanup(cleanup2)

	registry := daemon.NewRegistry()

	host1, portStr1, _ := net.SplitHostPort(addr1)
	port1 := 0
	fmt.Sscanf(portStr1, "%d", &port1)
	registry.Add(daemon.Device{
		ID: "device-1", Name: "Phone 1",
		TailscaleIP: host1, ListenPort: port1,
		CertFingerprint: "fp1", Source: daemon.SourceRuntimePaired,
	})

	host2, portStr2, _ := net.SplitHostPort(addr2)
	port2 := 0
	fmt.Sscanf(portStr2, "%d", &port2)
	registry.Add(daemon.Device{
		ID: "device-2", Name: "Phone 2",
		TailscaleIP: host2, ListenPort: port2,
		CertFingerprint: "fp2", Source: daemon.SourceRuntimePaired,
	})

	backend := agent.NewGRPCBackend(agent.GRPCBackendConfig{
		Registry:          registry,
		Dialer:            &testDialer{},
		AllowKeyListing:   true,
		ConnectionTimeout: 5 * time.Second,
		SignTimeout:       5 * time.Second,
	})

	keys, err := backend.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys from 2 devices, got %d", len(keys))
	}
}

// TestIntegrationGRPCBackendUnreachableDeviceSkipped verifies that
// unreachable devices are skipped during list-keys (FR-007).
func TestIntegrationGRPCBackendUnreachableDeviceSkipped(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Reachable phone.
	_, _, grpcKey := newTestECDSAKey(t, "reachable-key")
	phone := &testPhoneServer{keys: []*nixkeyv1.SSHKey{grpcKey}}
	addr, cleanup := startTestPhoneServer(t, phone)
	t.Cleanup(cleanup)

	registry := daemon.NewRegistry()

	host, portStr, _ := net.SplitHostPort(addr)
	port := 0
	fmt.Sscanf(portStr, "%d", &port)
	registry.Add(daemon.Device{
		ID: "reachable", Name: "Reachable Phone",
		TailscaleIP: host, ListenPort: port,
		CertFingerprint: "fp-reach", Source: daemon.SourceRuntimePaired,
	})

	// Unreachable phone (bad port).
	registry.Add(daemon.Device{
		ID: "unreachable", Name: "Unreachable Phone",
		TailscaleIP: "127.0.0.1", ListenPort: 1,
		CertFingerprint: "fp-unreach", Source: daemon.SourceRuntimePaired,
	})

	backend := agent.NewGRPCBackend(agent.GRPCBackendConfig{
		Registry:          registry,
		Dialer:            &testDialer{},
		AllowKeyListing:   true,
		ConnectionTimeout: 1 * time.Second,
		SignTimeout:       5 * time.Second,
	})

	keys, err := backend.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// Should get key from reachable phone, skip unreachable.
	if len(keys) != 1 {
		t.Fatalf("expected 1 key (unreachable skipped), got %d", len(keys))
	}
	if keys[0].Comment != "reachable-key" {
		t.Errorf("expected reachable-key, got %s", keys[0].Comment)
	}
}

// TestIntegrationGRPCBackendLastSeenUpdated verifies that LastSeen is
// updated on the device after a successful sign.
func TestIntegrationGRPCBackendLastSeenUpdated(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, signer, grpcKey := newTestECDSAKey(t, "lastseen-key")
	phone := &testPhoneServer{
		keys:   []*nixkeyv1.SSHKey{grpcKey},
		signer: signer,
	}

	backend, registry := setupTestBackend(t, phone)

	// Check initial state: lastSeen should be nil.
	dev, _ := registry.Get("test-device-1")
	if dev.LastSeen != nil {
		t.Fatalf("expected nil LastSeen initially, got %v", dev.LastSeen)
	}

	// List and sign.
	backend.List()
	pub, _ := ssh.ParsePublicKey(grpcKey.GetPublicKeyBlob())
	_, err := backend.Sign(pub, []byte("test"), 0)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	// LastSeen should now be set.
	dev, _ = registry.Get("test-device-1")
	if dev.LastSeen == nil {
		t.Fatal("expected LastSeen to be set after sign")
	}
}

// TestIntegrationGRPCBackendEndToEndWithSSHAgent tests the full flow:
// SSH agent server → GRPCBackend → in-process gRPC phone server.
func TestIntegrationGRPCBackendEndToEndWithSSHAgent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, signer, grpcKey := newTestECDSAKey(t, "e2e-key")
	phone := &testPhoneServer{
		keys:   []*nixkeyv1.SSHKey{grpcKey},
		signer: signer,
	}

	backend, _ := setupTestBackend(t, phone)

	// Start SSH agent with GRPCBackend.
	client, cleanup := startTestAgent(t, backend)
	defer cleanup()

	// List keys via SSH agent.
	keys, err := client.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	if keys[0].Comment != "e2e-key" {
		t.Errorf("expected comment 'e2e-key', got %q", keys[0].Comment)
	}

	// Sign via SSH agent.
	pub, err := ssh.ParsePublicKey(grpcKey.GetPublicKeyBlob())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	data := []byte("e2e sign test")
	sig, err := client.Sign(pub, data)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := pub.Verify(data, sig); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

