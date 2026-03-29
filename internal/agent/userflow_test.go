// userflow_test.go contains user-flow integration tests that exercise the
// full stack: SSH agent (Unix socket) → SSH client → GRPCBackend → in-process
// gRPC phone server. These tests cover SC-001, SC-006, FR-029, FR-E08.
package agent_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"testing"
	"time"

	sshagent "golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/phaedrus-raznikov/nix-key/internal/agent"
	"github.com/phaedrus-raznikov/nix-key/internal/daemon"

	nixkeyv1 "github.com/phaedrus-raznikov/nix-key/gen/nixkey/v1"
)

// --- User-Flow Integration Tests (T022) ---

// TestIntegrationUserFlowListAndSign tests the happy path: start agent on Unix
// socket, connect SSH client, list keys from in-process gRPC phone server,
// sign data, and verify the signature. (SC-001)
func TestIntegrationUserFlowListAndSign(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, signer, grpcKey := newTestECDSAKey(t, "user-flow-key")
	phone := &testPhoneServer{
		keys:   []*nixkeyv1.SSHKey{grpcKey},
		signer: signer,
	}

	backend, _ := setupTestBackend(t, phone)
	client, cleanup := startTestAgent(t, backend)
	defer cleanup()

	// ssh-add -L equivalent: list keys.
	keys, err := client.List()
	if err != nil {
		t.Fatalf("list keys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	if keys[0].Comment != "user-flow-key" {
		t.Errorf("expected comment 'user-flow-key', got %q", keys[0].Comment)
	}

	// Sign data via SSH agent protocol.
	pub, err := ssh.ParsePublicKey(grpcKey.GetPublicKeyBlob())
	if err != nil {
		t.Fatalf("parse public key: %v", err)
	}

	data := []byte("user flow sign test data")
	sig, err := client.Sign(pub, data)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	// Verify signature is cryptographically valid.
	if err := pub.Verify(data, sig); err != nil {
		t.Fatalf("verify signature: %v", err)
	}
}

// TestIntegrationUserFlowSignTimeout tests that when the phone server delays
// beyond signTimeout, the SSH client receives an error (SSH_AGENT_FAILURE). (SC-006)
func TestIntegrationUserFlowSignTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, signer, grpcKey := newTestECDSAKey(t, "timeout-flow-key")
	phone := &testPhoneServer{
		keys:      []*nixkeyv1.SSHKey{grpcKey},
		signer:    signer,
		signDelay: 10 * time.Second, // far exceeds signTimeout
	}

	backend, _ := setupTestBackend(t, phone, func(cfg *agent.GRPCBackendConfig) {
		cfg.SignTimeout = 200 * time.Millisecond
	})
	client, cleanup := startTestAgent(t, backend)
	defer cleanup()

	// Populate key cache via list.
	keys, err := client.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}

	pub, err := ssh.ParsePublicKey(grpcKey.GetPublicKeyBlob())
	if err != nil {
		t.Fatalf("parse public key: %v", err)
	}

	// Sign should fail due to timeout — SSH client sees generic failure.
	_, err = client.Sign(pub, []byte("timeout test"))
	if err == nil {
		t.Fatal("expected error from timed-out sign, got nil")
	}
}

// TestIntegrationUserFlowSignDenied tests that when the phone server rejects
// the sign request, the SSH client receives SSH_AGENT_FAILURE.
func TestIntegrationUserFlowSignDenied(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, _, grpcKey := newTestECDSAKey(t, "denied-flow-key")
	phone := &testPhoneServer{
		keys:     []*nixkeyv1.SSHKey{grpcKey},
		denySign: true,
	}

	backend, _ := setupTestBackend(t, phone)
	client, cleanup := startTestAgent(t, backend)
	defer cleanup()

	// List keys should still succeed.
	keys, err := client.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}

	pub, err := ssh.ParsePublicKey(grpcKey.GetPublicKeyBlob())
	if err != nil {
		t.Fatalf("parse public key: %v", err)
	}

	// Sign should fail due to denial — SSH client sees generic failure.
	_, err = client.Sign(pub, []byte("denied test"))
	if err == nil {
		t.Fatal("expected error from denied sign, got nil")
	}
}

// TestIntegrationUserFlowUnreachable tests that when no phone server is
// running, list-keys returns empty (fast fail, no hang).
func TestIntegrationUserFlowUnreachable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Register a device that points to a port with no server.
	registry := daemon.NewRegistry()
	registry.Add(daemon.Device{
		ID:              "ghost-device",
		Name:            "Ghost Phone",
		TailscaleIP:     "127.0.0.1",
		ListenPort:      1, // no server here
		CertFingerprint: fmt.Sprintf("%x", sha256.Sum256([]byte("ghost"))),
		Source:          daemon.SourceRuntimePaired,
	})

	backend := agent.NewGRPCBackend(agent.GRPCBackendConfig{
		Registry:          registry,
		Dialer:            &testDialer{},
		AllowKeyListing:   true,
		ConnectionTimeout: 500 * time.Millisecond, // fast timeout
		SignTimeout:       1 * time.Second,
	})

	client, cleanup := startTestAgent(t, backend)
	defer cleanup()

	start := time.Now()
	keys, err := client.List()
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("list should succeed (skip unreachable), got: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("expected 0 keys from unreachable device, got %d", len(keys))
	}
	// Verify fast fail — should not hang for a long time.
	if elapsed > 5*time.Second {
		t.Errorf("unreachable should fast-fail, took %v", elapsed)
	}
}

// droppingPhoneServer is a gRPC server that closes the connection mid-sign
// to simulate FR-E08 (mid-connection drop).
type droppingPhoneServer struct {
	nixkeyv1.UnimplementedNixKeyAgentServer
	keys     []*nixkeyv1.SSHKey
	listener net.Listener // used to force-close from Sign handler
}

func (s *droppingPhoneServer) ListKeys(_ context.Context, _ *nixkeyv1.ListKeysRequest) (*nixkeyv1.ListKeysResponse, error) {
	return &nixkeyv1.ListKeysResponse{Keys: s.keys}, nil
}

func (s *droppingPhoneServer) Sign(_ context.Context, _ *nixkeyv1.SignRequest) (*nixkeyv1.SignResponse, error) {
	// Force-close the listener to simulate a connection drop mid-RPC.
	s.listener.Close()
	// Return an error — the client may see this or a transport error,
	// depending on timing. Either way, the SSH agent should return failure.
	return nil, status.Error(codes.Unavailable, "connection lost")
}

func (s *droppingPhoneServer) Ping(_ context.Context, _ *nixkeyv1.PingRequest) (*nixkeyv1.PingResponse, error) {
	return &nixkeyv1.PingResponse{TimestampMs: time.Now().UnixMilli()}, nil
}

// TestIntegrationUserFlowMidConnectionDrop tests FR-E08: phone server closes
// connection mid-sign, SSH client must receive SSH_AGENT_FAILURE.
func TestIntegrationUserFlowMidConnectionDrop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, _, grpcKey := newTestECDSAKey(t, "drop-key")

	// Start a custom dropping phone server.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	dropServer := &droppingPhoneServer{
		keys:     []*nixkeyv1.SSHKey{grpcKey},
		listener: lis,
	}

	gs := grpc.NewServer()
	nixkeyv1.RegisterNixKeyAgentServer(gs, dropServer)
	go gs.Serve(lis)
	t.Cleanup(func() { gs.Stop() })

	// Set up registry and backend pointing to the dropping server.
	host, portStr, _ := net.SplitHostPort(lis.Addr().String())
	port := 0
	fmt.Sscanf(portStr, "%d", &port)

	registry := daemon.NewRegistry()
	registry.Add(daemon.Device{
		ID:              "drop-device",
		Name:            "Dropping Phone",
		TailscaleIP:     host,
		ListenPort:      port,
		CertFingerprint: fmt.Sprintf("%x", sha256.Sum256([]byte("drop-cert"))),
		Source:          daemon.SourceRuntimePaired,
	})

	backend := agent.NewGRPCBackend(agent.GRPCBackendConfig{
		Registry:          registry,
		Dialer:            &testDialer{},
		AllowKeyListing:   true,
		ConnectionTimeout: 5 * time.Second,
		SignTimeout:       5 * time.Second,
	})

	client, cleanup := startTestAgent(t, backend)
	defer cleanup()

	// List keys first to populate cache.
	keys, err := client.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}

	pub, err := ssh.ParsePublicKey(grpcKey.GetPublicKeyBlob())
	if err != nil {
		t.Fatalf("parse public key: %v", err)
	}

	// Sign should fail due to mid-connection drop — SSH_AGENT_FAILURE.
	_, err = client.Sign(pub, []byte("drop test"))
	if err == nil {
		t.Fatal("expected error from mid-connection drop, got nil")
	}
	// Error should not leak internal transport details.
	errMsg := err.Error()
	if containsStr(errMsg, "127.0.0.1") || containsStr(errMsg, "transport") {
		t.Errorf("error message leaked internal details: %s", errMsg)
	}
}

// TestIntegrationUserFlowMultiplePhones tests FR-029: multiple phones with
// distinct keys. List returns keys from all phones, sign routes to the correct
// device based on key fingerprint.
func TestIntegrationUserFlowMultiplePhones(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Phone 1: one ECDSA key.
	_, signer1, grpcKey1 := newTestECDSAKey(t, "phone1-key")
	phone1 := &testPhoneServer{
		keys:   []*nixkeyv1.SSHKey{grpcKey1},
		signer: signer1,
	}
	addr1, cleanup1 := startTestPhoneServer(t, phone1)
	t.Cleanup(cleanup1)

	// Phone 2: different ECDSA key.
	_, signer2, grpcKey2 := newTestECDSAKey(t, "phone2-key")
	phone2 := &testPhoneServer{
		keys:   []*nixkeyv1.SSHKey{grpcKey2},
		signer: signer2,
	}
	addr2, cleanup2 := startTestPhoneServer(t, phone2)
	t.Cleanup(cleanup2)

	// Build registry with both devices.
	registry := daemon.NewRegistry()

	host1, portStr1, _ := net.SplitHostPort(addr1)
	port1 := 0
	fmt.Sscanf(portStr1, "%d", &port1)
	registry.Add(daemon.Device{
		ID: "phone-1", Name: "Phone 1",
		TailscaleIP: host1, ListenPort: port1,
		CertFingerprint: fmt.Sprintf("%x", sha256.Sum256([]byte("phone1-cert"))),
		Source:          daemon.SourceRuntimePaired,
	})

	host2, portStr2, _ := net.SplitHostPort(addr2)
	port2 := 0
	fmt.Sscanf(portStr2, "%d", &port2)
	registry.Add(daemon.Device{
		ID: "phone-2", Name: "Phone 2",
		TailscaleIP: host2, ListenPort: port2,
		CertFingerprint: fmt.Sprintf("%x", sha256.Sum256([]byte("phone2-cert"))),
		Source:          daemon.SourceRuntimePaired,
	})

	backend := agent.NewGRPCBackend(agent.GRPCBackendConfig{
		Registry:          registry,
		Dialer:            &testDialer{},
		AllowKeyListing:   true,
		ConnectionTimeout: 5 * time.Second,
		SignTimeout:       5 * time.Second,
	})

	client, cleanup := startTestAgent(t, backend)
	defer cleanup()

	// List keys from both phones.
	keys, err := client.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys from 2 phones, got %d", len(keys))
	}

	// Verify both phone keys are present.
	names := make(map[string]bool)
	for _, k := range keys {
		names[k.Comment] = true
	}
	if !names["phone1-key"] || !names["phone2-key"] {
		t.Errorf("expected both phone keys, got %v", names)
	}

	// Sign with phone 1's key.
	pub1, err := ssh.ParsePublicKey(grpcKey1.GetPublicKeyBlob())
	if err != nil {
		t.Fatalf("parse phone1 key: %v", err)
	}
	data1 := []byte("data for phone 1")
	sig1, err := client.Sign(pub1, data1)
	if err != nil {
		t.Fatalf("sign with phone1 key: %v", err)
	}
	if err := pub1.Verify(data1, sig1); err != nil {
		t.Fatalf("verify phone1 signature: %v", err)
	}

	// Sign with phone 2's key — routes to the other device.
	pub2, err := ssh.ParsePublicKey(grpcKey2.GetPublicKeyBlob())
	if err != nil {
		t.Fatalf("parse phone2 key: %v", err)
	}
	data2 := []byte("data for phone 2")
	sig2, err := client.Sign(pub2, data2)
	if err != nil {
		t.Fatalf("sign with phone2 key: %v", err)
	}
	if err := pub2.Verify(data2, sig2); err != nil {
		t.Fatalf("verify phone2 signature: %v", err)
	}

	// Verify each phone only handled its own sign request.
	if got := phone1.signCount.Load(); got != 1 {
		t.Errorf("phone1 expected 1 sign call, got %d", got)
	}
	if got := phone2.signCount.Load(); got != 1 {
		t.Errorf("phone2 expected 1 sign call, got %d", got)
	}
}

// TestIntegrationUserFlowSignWithFlags tests that SignWithFlags (algorithm
// negotiation via ExtendedAgent) works through the full stack.
func TestIntegrationUserFlowSignWithFlags(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, signer, grpcKey := newTestECDSAKey(t, "flags-key")
	phone := &testPhoneServer{
		keys:   []*nixkeyv1.SSHKey{grpcKey},
		signer: signer,
	}

	backend, _ := setupTestBackend(t, phone)
	client, cleanup := startTestAgent(t, backend)
	defer cleanup()

	keys, err := client.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}

	pub, err := ssh.ParsePublicKey(grpcKey.GetPublicKeyBlob())
	if err != nil {
		t.Fatalf("parse public key: %v", err)
	}

	data := []byte("flags sign test")
	sig, err := client.SignWithFlags(pub, data, sshagent.SignatureFlagReserved)
	if err != nil {
		t.Fatalf("sign with flags: %v", err)
	}
	if err := pub.Verify(data, sig); err != nil {
		t.Fatalf("verify flagged signature: %v", err)
	}
}
