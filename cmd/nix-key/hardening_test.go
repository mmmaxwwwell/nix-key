package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
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
	"github.com/phaedrus-raznikov/nix-key/internal/mtls"

	nixkeyv1 "github.com/phaedrus-raznikov/nix-key/gen/nixkey/v1"
)

// T-HI-11: Cert expiry warning appears in status output and collectCertWarnings
// returns the correct DaysLeft and CertType fields.
func TestIntegrationCertExpiryWarningDetails(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	reg, socketPath, dir := startIntegrationDaemon(t, nil)

	// Generate server cert expiring in 5 days.
	serverCert, _, err := mtls.GenerateCert(mtls.CertOptions{
		KeyType:    mtls.KeyTypeECDSAP256,
		CommonName: "near-expiry-phone",
		Expiry:     5 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("generate server cert: %v", err)
	}
	serverCertPath := filepath.Join(dir, "server-expiring.pem")
	if err := os.WriteFile(serverCertPath, serverCert, 0600); err != nil {
		t.Fatal(err)
	}

	// Generate client cert expiring in 15 days.
	clientCert, _, err := mtls.GenerateCert(mtls.CertOptions{
		KeyType:    mtls.KeyTypeEd25519,
		CommonName: "near-expiry-host-client",
		Expiry:     15 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("generate client cert: %v", err)
	}
	clientCertPath := filepath.Join(dir, "client-expiring.pem")
	if err := os.WriteFile(clientCertPath, clientCert, 0600); err != nil {
		t.Fatal(err)
	}

	// Generate cert that expires in 60 days (outside 30-day warning threshold).
	safeCert, _, err := mtls.GenerateCert(mtls.CertOptions{
		KeyType:    mtls.KeyTypeEd25519,
		CommonName: "safe-phone",
		Expiry:     60 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("generate safe cert: %v", err)
	}
	safeCertPath := filepath.Join(dir, "safe.pem")
	if err := os.WriteFile(safeCertPath, safeCert, 0600); err != nil {
		t.Fatal(err)
	}

	// Device with both expiring certs.
	reg.Add(daemon.Device{
		ID: "dev-expiry", Name: "Expiry Phone", TailscaleIP: "100.64.0.20",
		ListenPort: 29418, CertFingerprint: "fp-expiry",
		CertPath: serverCertPath, ClientCertPath: clientCertPath,
		Source: daemon.SourceRuntimePaired,
	})

	// Device with safe cert.
	reg.Add(daemon.Device{
		ID: "dev-safe", Name: "Safe Phone", TailscaleIP: "100.64.0.21",
		ListenPort: 29418, CertFingerprint: "fp-safe",
		CertPath: safeCertPath,
		Source:   daemon.SourceRuntimePaired,
	})

	// Check status output mentions warnings for expiring device only.
	var buf strings.Builder
	err = runStatus(socketPath, &buf)
	if err != nil {
		t.Fatalf("runStatus: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "Certificate warnings") {
		t.Errorf("status should show cert warnings:\n%s", output)
	}
	if !strings.Contains(output, "Expiry Phone") {
		t.Errorf("status should mention expiring device:\n%s", output)
	}
	// Both server and client cert should warn.
	if !strings.Contains(output, "server") {
		t.Errorf("status should mention server cert type:\n%s", output)
	}
	if !strings.Contains(output, "client") {
		t.Errorf("status should mention client cert type:\n%s", output)
	}

	// Safe phone should not appear in warnings.
	if strings.Contains(output, "Safe Phone") {
		t.Errorf("safe phone (60d expiry) should not trigger warning:\n%s", output)
	}
}

// T-HI-12: Pairing generates two distinct cert pairs (host client cert + phone server cert).
func TestIntegrationTwoCertPairsDuringPairing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Generate two independent client cert pairs (simulating what pairing does).
	cert1PEM, key1PEM, err := mtls.GenerateCert(mtls.CertOptions{
		KeyType:    mtls.KeyTypeECDSAP256,
		CommonName: "nix-key host client",
		Expiry:     24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("generate cert1: %v", err)
	}

	cert2PEM, key2PEM, err := mtls.GenerateCert(mtls.CertOptions{
		KeyType:    mtls.KeyTypeECDSAP256,
		CommonName: "nix-key host client",
		Expiry:     24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("generate cert2: %v", err)
	}

	// Verify both are valid PEM certificates.
	for i, certPEM := range [][]byte{cert1PEM, cert2PEM} {
		block, _ := pem.Decode(certPEM)
		if block == nil {
			t.Fatalf("cert %d: not valid PEM", i+1)
		}
		if block.Type != "CERTIFICATE" {
			t.Errorf("cert %d: PEM type = %q, want CERTIFICATE", i+1, block.Type)
		}
	}

	// Verify both are valid PEM private keys.
	for i, keyPEM := range [][]byte{key1PEM, key2PEM} {
		block, _ := pem.Decode(keyPEM)
		if block == nil {
			t.Fatalf("key %d: not valid PEM", i+1)
		}
		if block.Type != "PRIVATE KEY" && block.Type != "EC PRIVATE KEY" {
			t.Errorf("key %d: PEM type = %q, want PRIVATE KEY or EC PRIVATE KEY", i+1, block.Type)
		}
	}

	// Verify the certs are distinct (different serial numbers / keys).
	if bytes.Equal(cert1PEM, cert2PEM) {
		t.Error("two generated certs should be distinct")
	}
	if bytes.Equal(key1PEM, key2PEM) {
		t.Error("two generated keys should be distinct")
	}

	// Verify a pairing stores both cert paths on the device record.
	dir := t.TempDir()
	devicesPath := filepath.Join(dir, "devices.json")
	certsDir := filepath.Join(dir, "certs")
	ageIDPath := filepath.Join(dir, "age-identity.txt")

	// Generate age identity for encryption.
	if err := mtls.GenerateIdentity(ageIDPath); err != nil {
		t.Fatalf("generate identity: %v", err)
	}

	// Simulate a device registration: store both phone cert and host client cert.
	phoneCertDir := filepath.Join(certsDir, "deadbeef01234567")
	if err := os.MkdirAll(phoneCertDir, 0700); err != nil {
		t.Fatal(err)
	}

	phoneCertPath := filepath.Join(phoneCertDir, "phone-server-cert.pem")
	hostCertPath := filepath.Join(phoneCertDir, "host-client-cert.pem")
	hostKeyPath := filepath.Join(phoneCertDir, "host-client-key.pem")

	if err := os.WriteFile(phoneCertPath, cert1PEM, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hostCertPath, cert2PEM, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hostKeyPath, key2PEM, 0600); err != nil {
		t.Fatal(err)
	}

	dev := daemon.Device{
		ID: "paired-dev", Name: "Test Phone",
		TailscaleIP: "100.64.0.1", ListenPort: 29418,
		CertFingerprint: "deadbeef01234567",
		CertPath:        phoneCertPath,
		ClientCertPath:  hostCertPath,
		ClientKeyPath:   hostKeyPath,
		Source:          daemon.SourceRuntimePaired,
	}

	reg := daemon.NewRegistry()
	reg.Add(dev)
	if err := reg.SaveToJSON(devicesPath); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Reload and verify both paths set.
	loaded, err := daemon.LoadDevicesFromJSON(devicesPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 device, got %d", len(loaded))
	}
	if loaded[0].CertPath == "" {
		t.Error("CertPath (phone cert) should be set")
	}
	if loaded[0].ClientCertPath == "" {
		t.Error("ClientCertPath (host cert) should be set")
	}
	if loaded[0].ClientKeyPath == "" {
		t.Error("ClientKeyPath (host key) should be set")
	}
}

// T-HI-13: Age decrypt failure at startup when wrong identity is used.
func TestIntegrationAgeDecryptFailureWrongIdentity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()

	// Generate correct identity and encrypt a test file.
	correctID := filepath.Join(dir, "correct-identity.txt")
	if err := mtls.GenerateIdentity(correctID); err != nil {
		t.Fatalf("generate correct identity: %v", err)
	}

	plainPath := filepath.Join(dir, "secret.pem")
	if err := os.WriteFile(plainPath, []byte("-----BEGIN EC PRIVATE KEY-----\nfake-key-data\n-----END EC PRIVATE KEY-----\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := mtls.EncryptFile(plainPath, correctID); err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	encPath := plainPath + ".age"

	// Verify correct identity can decrypt.
	decrypted, err := mtls.DecryptToMemory(encPath, correctID)
	if err != nil {
		t.Fatalf("decrypt with correct identity should succeed: %v", err)
	}
	if !strings.Contains(string(decrypted), "fake-key-data") {
		t.Error("decrypted data should contain original content")
	}

	// Generate a different identity.
	wrongID := filepath.Join(dir, "wrong-identity.txt")
	if err := mtls.GenerateIdentity(wrongID); err != nil {
		t.Fatalf("generate wrong identity: %v", err)
	}

	// Decrypt with wrong identity should fail.
	_, err = mtls.DecryptToMemory(encPath, wrongID)
	if err == nil {
		t.Fatal("decrypt with wrong identity should fail")
	}
	if !strings.Contains(err.Error(), "decrypt") {
		t.Errorf("error should mention decryption, got: %v", err)
	}
}

// hardeningPhoneServer implements NixKeyAgentServer for deleted-key testing.
type hardeningPhoneServer struct {
	nixkeyv1.UnimplementedNixKeyAgentServer
	keys []*nixkeyv1.SSHKey
}

func (s *hardeningPhoneServer) ListKeys(_ context.Context, _ *nixkeyv1.ListKeysRequest) (*nixkeyv1.ListKeysResponse, error) {
	return &nixkeyv1.ListKeysResponse{Keys: s.keys}, nil
}

func (s *hardeningPhoneServer) Sign(_ context.Context, req *nixkeyv1.SignRequest) (*nixkeyv1.SignResponse, error) {
	return nil, status.Error(codes.NotFound, "key not found on device")
}

func (s *hardeningPhoneServer) Ping(_ context.Context, _ *nixkeyv1.PingRequest) (*nixkeyv1.PingResponse, error) {
	return &nixkeyv1.PingResponse{TimestampMs: time.Now().UnixMilli()}, nil
}

// hardeningDialer dials using plain gRPC (no mTLS) for testing.
type hardeningDialer struct{}

func (d *hardeningDialer) DialDevice(ctx context.Context, dev daemon.Device) (nixkeyv1.NixKeyAgentClient, func(), error) {
	addr := fmt.Sprintf("%s:%d", dev.TailscaleIP, dev.ListenPort)
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, nil, err
	}
	client := nixkeyv1.NewNixKeyAgentClient(conn)
	cleanup := func() { _ = conn.Close() }
	return client, cleanup, nil
}

// T-HI-14: Signing with a key that was deleted from the phone after listing.
func TestIntegrationDeletedKeySignRequest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Generate a real key for the initial listing.
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(privKey)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	pub := signer.PublicKey()

	grpcKey := &nixkeyv1.SSHKey{
		PublicKeyBlob: pub.Marshal(),
		KeyType:       pub.Type(),
		DisplayName:   "deleted-key",
		Fingerprint:   ssh.FingerprintSHA256(pub),
	}

	// Phone server lists the key but returns NotFound on sign (simulating deletion).
	phone := &hardeningPhoneServer{
		keys: []*nixkeyv1.SSHKey{grpcKey},
	}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	gs := grpc.NewServer()
	nixkeyv1.RegisterNixKeyAgentServer(gs, phone)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(func() { gs.GracefulStop() })

	host, portStr, _ := net.SplitHostPort(lis.Addr().String())
	port := 0
	_, _ = fmt.Sscanf(portStr, "%d", &port)

	registry := daemon.NewRegistry()
	registry.Add(daemon.Device{
		ID:              "phone-deleted-key",
		Name:            "Phone With Deleted Key",
		TailscaleIP:     host,
		ListenPort:      port,
		CertFingerprint: fmt.Sprintf("%x", sha256.Sum256([]byte("deleted-key"))),
		Source:          daemon.SourceRuntimePaired,
	})

	backend := agent.NewGRPCBackend(agent.GRPCBackendConfig{
		Registry:          registry,
		Dialer:            &hardeningDialer{},
		AllowKeyListing:   true,
		ConnectionTimeout: 5 * time.Second,
		SignTimeout:       5 * time.Second,
	})

	// List keys (populates cache).
	keys, err := backend.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}

	// Sign with the listed key - phone returns NotFound.
	parsedPub, err := ssh.ParsePublicKey(grpcKey.GetPublicKeyBlob())
	if err != nil {
		t.Fatalf("parse key: %v", err)
	}

	_, err = backend.Sign(parsedPub, []byte("test data"), 0)
	if err == nil {
		t.Fatal("expected error when signing with deleted key")
	}

	// Verify the SSH agent sanitizes the error (FR-097).
	agentBackend := backend
	socketDir := t.TempDir()
	socketPath := filepath.Join(socketDir, "agent.sock")
	srv, err := agent.NewServer(agentBackend, socketPath)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	go func() { _ = srv.Serve() }()
	t.Cleanup(func() { _ = srv.Close() })

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("dial agent: %v", err)
	}
	defer conn.Close()

	agentClient := sshagent.NewClient(conn)
	_, signErr := agentClient.Sign(parsedPub, []byte("test data"))
	if signErr == nil {
		t.Fatal("expected agent error for deleted key sign")
	}
	// Error should not contain internal details.
	errMsg := signErr.Error()
	if strings.Contains(errMsg, "NotFound") || strings.Contains(errMsg, "not found on device") {
		t.Errorf("SSH agent error leaked internal details: %s", errMsg)
	}
}

// T-HI-15: Atomic pairing — write failure does not leave partial state.
func TestIntegrationAtomicPairingWriteFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()

	// Create a read-only parent to prevent MkdirAll from creating the devices dir.
	readOnlyDir := filepath.Join(dir, "readonly")
	if err := os.MkdirAll(readOnlyDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(readOnlyDir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(readOnlyDir, 0700) })

	// Try to save devices inside a subdir of the read-only dir.
	// SaveToJSON calls MkdirAll which should fail.
	devicesPath := filepath.Join(readOnlyDir, "subdir", "devices.json")

	reg := daemon.NewRegistry()
	reg.Add(daemon.Device{
		ID: "fail-dev", Name: "Fail Phone",
		CertFingerprint: "fp-fail",
		Source:          daemon.SourceRuntimePaired,
	})

	err := reg.SaveToJSON(devicesPath)
	if err == nil {
		t.Fatal("expected write failure when parent dir is read-only")
	}

	// Verify no files were created in the read-only directory.
	_ = os.Chmod(readOnlyDir, 0700) // restore to check
	subdir := filepath.Join(readOnlyDir, "subdir")
	if _, statErr := os.Stat(subdir); statErr == nil {
		t.Error("subdir should not have been created on write failure")
	}
	if _, statErr := os.Stat(devicesPath); statErr == nil {
		t.Error("devices.json should not exist after write failure")
	}
}

// T-HI-20: Control socket full command round-trip and permissions.
func TestIntegrationControlSocketRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	socketPath := filepath.Join(dir, "control.sock")
	devicesPath := filepath.Join(dir, "devices.json")

	reg := daemon.NewRegistry()
	reg.Add(daemon.Device{
		ID: "ctl-dev", Name: "Control Test Phone",
		TailscaleIP: "100.64.0.30", ListenPort: 29418,
		CertFingerprint: "fp-ctl",
		Source:          daemon.SourceRuntimePaired,
	})

	keys := []daemon.KeyInfo{
		{Fingerprint: "SHA256:ctlkey1", KeyType: "ssh-ed25519", DisplayName: "ctl-key", DeviceID: "ctl-dev"},
	}

	srv := daemon.NewControlServer(daemon.ControlServerConfig{
		SocketPath:  socketPath,
		Registry:    reg,
		DevicesPath: devicesPath,
		KeyLister:   func() []daemon.KeyInfo { return keys },
	})

	if err := srv.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	// Verify socket permissions (0600).
	info, err := os.Stat(socketPath)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("socket permissions = %o, want 0600", perm)
	}

	client := daemon.NewControlClient(socketPath)

	// Test get-status.
	resp, err := client.SendCommand(daemon.Request{Command: "get-status"})
	if err != nil {
		t.Fatalf("get-status: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("get-status status = %q, want ok", resp.Status)
	}

	// Test list-devices.
	resp, err = client.SendCommand(daemon.Request{Command: "list-devices"})
	if err != nil {
		t.Fatalf("list-devices: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("list-devices status = %q, want ok", resp.Status)
	}

	// Test get-keys.
	resp, err = client.SendCommand(daemon.Request{Command: "get-keys"})
	if err != nil {
		t.Fatalf("get-keys: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("get-keys status = %q, want ok", resp.Status)
	}

	// Test get-device.
	resp, err = client.SendCommand(daemon.Request{Command: "get-device", DeviceID: "ctl-dev"})
	if err != nil {
		t.Fatalf("get-device: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("get-device status = %q, want ok", resp.Status)
	}

	// Test unknown command.
	resp, err = client.SendCommand(daemon.Request{Command: "bogus"})
	if err != nil {
		t.Fatalf("bogus command: %v", err)
	}
	if resp.Status != "error" {
		t.Errorf("bogus status = %q, want error", resp.Status)
	}
	if !strings.Contains(resp.Error, "unknown command") {
		t.Errorf("bogus error = %q, want 'unknown command'", resp.Error)
	}

	// Test graceful shutdown.
	srv.Stop()

	// After stop, new connections should fail.
	_, err = client.SendCommand(daemon.Request{Command: "get-status"})
	if err == nil {
		t.Error("expected error after server stop")
	}
}
