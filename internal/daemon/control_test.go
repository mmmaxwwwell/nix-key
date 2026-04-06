package daemon_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	nixkeyv1 "github.com/phaedrus-raznikov/nix-key/gen/nixkey/v1"
	"github.com/phaedrus-raznikov/nix-key/internal/daemon"
	"github.com/phaedrus-raznikov/nix-key/internal/mtls"
	"google.golang.org/grpc"
)

// sendCommand dials the control socket, sends a JSON command, and reads the response.
func sendCommand(t *testing.T, socketPath string, req daemon.Request) daemon.Response {
	t.Helper()

	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		t.Fatalf("dial control socket: %v", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	_, err = fmt.Fprintf(conn, "%s\n", data)
	if err != nil {
		t.Fatalf("write request: %v", err)
	}

	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		t.Fatalf("no response from control socket: %v", scanner.Err())
	}

	var resp daemon.Response
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response %q: %v", scanner.Text(), err)
	}
	return resp
}

// startTestControlServer creates a registry, starts a control server, and returns them.
func startTestControlServer(t *testing.T) (*daemon.Registry, string) {
	t.Helper()

	dir, err := os.MkdirTemp("/tmp", "nk")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	socketPath := filepath.Join(dir, "control.sock")
	devicesPath := filepath.Join(dir, "devices.json")

	reg := daemon.NewRegistry()

	srv := daemon.NewControlServer(daemon.ControlServerConfig{
		SocketPath:  socketPath,
		Registry:    reg,
		DevicesPath: devicesPath,
		KeyLister:   func() []daemon.KeyInfo { return nil },
	})

	if err := srv.Start(); err != nil {
		t.Fatalf("start control server: %v", err)
	}
	t.Cleanup(func() { srv.Stop() })

	return reg, socketPath
}

func TestControlRegisterDevice(t *testing.T) {
	reg, socketPath := startTestControlServer(t)

	// First, add a device via the registry and save to disk so register-device can reload.
	dev := daemon.Device{
		ID:              "dev-001",
		Name:            "Test Phone",
		TailscaleIP:     "100.64.0.2",
		ListenPort:      29418,
		CertFingerprint: "abc123",
		Source:          daemon.SourceRuntimePaired,
	}
	reg.Add(dev)

	// Save so that register-device can reload from disk.
	dir := filepath.Dir(socketPath)
	devicesPath := filepath.Join(dir, "devices.json")
	if err := reg.SaveToJSON(devicesPath); err != nil {
		t.Fatalf("save devices: %v", err)
	}

	// Now remove from registry to simulate daemon not knowing about the device.
	reg.Remove("dev-001")

	resp := sendCommand(t, socketPath, daemon.Request{
		Command:  "register-device",
		DeviceID: "dev-001",
	})

	if resp.Status != "ok" {
		t.Errorf("expected status ok, got %q (error: %s)", resp.Status, resp.Error)
	}

	// Device should now be in registry after reload.
	if _, ok := reg.Get("dev-001"); !ok {
		t.Error("device dev-001 not found in registry after register-device")
	}
}

func TestControlListDevices(t *testing.T) {
	reg, socketPath := startTestControlServer(t)

	now := time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)
	reg.Add(daemon.Device{
		ID:              "dev-a",
		Name:            "Phone A",
		TailscaleIP:     "100.64.0.2",
		ListenPort:      29418,
		CertFingerprint: "fp-a",
		LastSeen:        &now,
		Source:          daemon.SourceRuntimePaired,
	})
	reg.Add(daemon.Device{
		ID:              "dev-b",
		Name:            "Phone B",
		TailscaleIP:     "100.64.0.3",
		ListenPort:      29418,
		CertFingerprint: "fp-b",
		Source:          daemon.SourceNixDeclared,
	})

	resp := sendCommand(t, socketPath, daemon.Request{Command: "list-devices"})

	if resp.Status != "ok" {
		t.Fatalf("expected status ok, got %q (error: %s)", resp.Status, resp.Error)
	}

	// Data should be a list of devices.
	data, err := json.Marshal(resp.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}

	var devices []daemon.DeviceInfo
	if err := json.Unmarshal(data, &devices); err != nil {
		t.Fatalf("unmarshal devices: %v", err)
	}

	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}

	// Verify fields are present.
	found := map[string]bool{}
	for _, d := range devices {
		found[d.ID] = true
		if d.Name == "" {
			t.Errorf("device %s has empty name", d.ID)
		}
	}
	if !found["dev-a"] || !found["dev-b"] {
		t.Errorf("expected both dev-a and dev-b in list, got %v", found)
	}
}

func TestControlRevokeDevice(t *testing.T) {
	reg, socketPath := startTestControlServer(t)

	reg.Add(daemon.Device{
		ID:              "dev-revoke",
		Name:            "Revoke Me",
		TailscaleIP:     "100.64.0.5",
		ListenPort:      29418,
		CertFingerprint: "fp-revoke",
		Source:          daemon.SourceRuntimePaired,
	})

	resp := sendCommand(t, socketPath, daemon.Request{
		Command:  "revoke-device",
		DeviceID: "dev-revoke",
	})

	if resp.Status != "ok" {
		t.Errorf("expected status ok, got %q (error: %s)", resp.Status, resp.Error)
	}

	if _, ok := reg.Get("dev-revoke"); ok {
		t.Error("device should have been removed from registry")
	}
}

func TestControlRevokeNixDeclaredDevice(t *testing.T) {
	reg, socketPath := startTestControlServer(t)

	reg.Add(daemon.Device{
		ID:              "nix-dev",
		Name:            "Nix Device",
		TailscaleIP:     "100.64.0.6",
		ListenPort:      29418,
		CertFingerprint: "fp-nix",
		Source:          daemon.SourceNixDeclared,
	})

	resp := sendCommand(t, socketPath, daemon.Request{
		Command:  "revoke-device",
		DeviceID: "nix-dev",
	})

	if resp.Status != "error" {
		t.Errorf("expected status error for nix-declared device, got %q", resp.Status)
	}

	if _, ok := reg.Get("nix-dev"); !ok {
		t.Error("nix-declared device should NOT have been removed")
	}
}

func TestControlRevokeDeletesCertFiles(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "control.sock")
	devicesPath := filepath.Join(dir, "devices.json")

	reg := daemon.NewRegistry()

	srv := daemon.NewControlServer(daemon.ControlServerConfig{
		SocketPath:  socketPath,
		Registry:    reg,
		DevicesPath: devicesPath,
		KeyLister:   func() []daemon.KeyInfo { return nil },
	})

	if err := srv.Start(); err != nil {
		t.Fatalf("start control server: %v", err)
	}
	t.Cleanup(func() { srv.Stop() })

	// Create cert files on disk.
	certDir := filepath.Join(dir, "certs", "abc123def456")
	if err := os.MkdirAll(certDir, 0700); err != nil {
		t.Fatalf("mkdir cert dir: %v", err)
	}

	phoneCertPath := filepath.Join(certDir, "phone-server-cert.pem")
	clientCertPath := filepath.Join(certDir, "host-client-cert.pem")
	clientKeyPath := filepath.Join(certDir, "host-client-key.pem.age")

	for _, p := range []string{phoneCertPath, clientCertPath, clientKeyPath} {
		if err := os.WriteFile(p, []byte("test-data"), 0600); err != nil {
			t.Fatalf("write cert file: %v", err)
		}
	}

	reg.Add(daemon.Device{
		ID:              "dev-with-certs",
		Name:            "Phone With Certs",
		TailscaleIP:     "100.64.0.10",
		ListenPort:      29418,
		CertFingerprint: "abc123def456",
		CertPath:        phoneCertPath,
		ClientCertPath:  clientCertPath,
		ClientKeyPath:   clientKeyPath,
		Source:          daemon.SourceRuntimePaired,
	})

	resp := sendCommand(t, socketPath, daemon.Request{
		Command:  "revoke-device",
		DeviceID: "dev-with-certs",
	})

	if resp.Status != "ok" {
		t.Fatalf("expected status ok, got %q (error: %s)", resp.Status, resp.Error)
	}

	// Verify cert files were deleted.
	for _, p := range []string{phoneCertPath, clientCertPath, clientKeyPath} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("cert file %s should have been deleted", filepath.Base(p))
		}
	}

	// Verify cert directory was removed.
	if _, err := os.Stat(certDir); err == nil {
		t.Error("cert directory should have been removed")
	}
}

func TestControlRevokeNonexistentDevice(t *testing.T) {
	_, socketPath := startTestControlServer(t)

	resp := sendCommand(t, socketPath, daemon.Request{
		Command:  "revoke-device",
		DeviceID: "does-not-exist",
	})

	if resp.Status != "error" {
		t.Errorf("expected status error for nonexistent device, got %q", resp.Status)
	}
}

func TestControlGetStatus(t *testing.T) {
	reg, socketPath := startTestControlServer(t)

	reg.Add(daemon.Device{
		ID:              "dev-status",
		Name:            "Status Phone",
		TailscaleIP:     "100.64.0.7",
		ListenPort:      29418,
		CertFingerprint: "fp-status",
		Source:          daemon.SourceRuntimePaired,
	})

	resp := sendCommand(t, socketPath, daemon.Request{Command: "get-status"})

	if resp.Status != "ok" {
		t.Fatalf("expected status ok, got %q (error: %s)", resp.Status, resp.Error)
	}

	data, err := json.Marshal(resp.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}

	var status daemon.StatusInfo
	if err := json.Unmarshal(data, &status); err != nil {
		t.Fatalf("unmarshal status: %v", err)
	}

	if !status.Running {
		t.Error("expected running=true")
	}
	if status.DeviceCount != 1 {
		t.Errorf("expected 1 device, got %d", status.DeviceCount)
	}
	if status.SocketPath != socketPath {
		t.Errorf("expected socketPath=%s, got %s", socketPath, status.SocketPath)
	}
}

func TestControlGetKeys(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "control.sock")
	devicesPath := filepath.Join(dir, "devices.json")

	reg := daemon.NewRegistry()

	testKeys := []daemon.KeyInfo{
		{Fingerprint: "SHA256:abc", KeyType: "ssh-ed25519", DisplayName: "test-key-1", DeviceID: "dev-1"},
		{Fingerprint: "SHA256:def", KeyType: "ecdsa-sha2-nistp256", DisplayName: "test-key-2", DeviceID: "dev-2"},
	}

	srv := daemon.NewControlServer(daemon.ControlServerConfig{
		SocketPath:  socketPath,
		Registry:    reg,
		DevicesPath: devicesPath,
		KeyLister:   func() []daemon.KeyInfo { return testKeys },
	})

	if err := srv.Start(); err != nil {
		t.Fatalf("start control server: %v", err)
	}
	defer srv.Stop()

	resp := sendCommand(t, socketPath, daemon.Request{Command: "get-keys"})

	if resp.Status != "ok" {
		t.Fatalf("expected status ok, got %q (error: %s)", resp.Status, resp.Error)
	}

	data, err := json.Marshal(resp.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}

	var keys []daemon.KeyInfo
	if err := json.Unmarshal(data, &keys); err != nil {
		t.Fatalf("unmarshal keys: %v", err)
	}

	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
	if keys[0].Fingerprint != "SHA256:abc" {
		t.Errorf("expected first key fingerprint SHA256:abc, got %s", keys[0].Fingerprint)
	}
}

func TestControlUnknownCommand(t *testing.T) {
	_, socketPath := startTestControlServer(t)

	resp := sendCommand(t, socketPath, daemon.Request{Command: "nonexistent"})

	if resp.Status != "error" {
		t.Errorf("expected status error for unknown command, got %q", resp.Status)
	}
}

func TestControlClientSendCommand(t *testing.T) {
	reg, socketPath := startTestControlServer(t)

	reg.Add(daemon.Device{
		ID:              "dev-client",
		Name:            "Client Phone",
		TailscaleIP:     "100.64.0.8",
		ListenPort:      29418,
		CertFingerprint: "fp-client",
		Source:          daemon.SourceRuntimePaired,
	})

	// Test using the client helper.
	client := daemon.NewControlClient(socketPath)
	resp, err := client.SendCommand(daemon.Request{Command: "get-status"})
	if err != nil {
		t.Fatalf("client send command: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("expected status ok, got %q", resp.Status)
	}
}

// TestIntegrationRevokedCertRejectedOnMTLS verifies FR-E09: after revocation,
// the revoked device's cert files are deleted and an mTLS handshake attempt fails.
func TestIntegrationRevokedCertRejectedOnMTLS(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir, err := os.MkdirTemp("/tmp", "nk")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	socketPath := filepath.Join(dir, "control.sock")
	devicesPath := filepath.Join(dir, "devices.json")

	reg := daemon.NewRegistry()

	srv := daemon.NewControlServer(daemon.ControlServerConfig{
		SocketPath:  socketPath,
		Registry:    reg,
		DevicesPath: devicesPath,
		KeyLister:   func() []daemon.KeyInfo { return nil },
	})
	if err := srv.Start(); err != nil {
		t.Fatalf("start control server: %v", err)
	}
	t.Cleanup(func() { srv.Stop() })

	// Generate real mTLS certs (phone=server, host=client).
	phoneCertPEM, phoneKeyPEM, err := mtls.GenerateCert(mtls.CertOptions{
		KeyType:    mtls.KeyTypeEd25519,
		CommonName: "phone-server",
	})
	if err != nil {
		t.Fatalf("generate phone cert: %v", err)
	}
	hostCertPEM, hostKeyPEM, err := mtls.GenerateCert(mtls.CertOptions{
		KeyType:    mtls.KeyTypeEd25519,
		CommonName: "host-client",
	})
	if err != nil {
		t.Fatalf("generate host cert: %v", err)
	}

	phoneFP, _ := mtls.CertFingerprint(phoneCertPEM)
	hostFP, _ := mtls.CertFingerprint(hostCertPEM)

	// Write cert files to a device cert directory.
	certDir := filepath.Join(dir, "certs", phoneFP[:16])
	if err := os.MkdirAll(certDir, 0700); err != nil {
		t.Fatalf("mkdir cert dir: %v", err)
	}

	phoneCertPath := filepath.Join(certDir, "phone-server-cert.pem")
	hostCertPath := filepath.Join(certDir, "host-client-cert.pem")
	hostKeyPath := filepath.Join(certDir, "host-client-key.pem")
	phoneKeyPath := filepath.Join(dir, "phone-server-key.pem") // kept outside certDir

	for path, data := range map[string][]byte{
		phoneCertPath: phoneCertPEM,
		hostCertPath:  hostCertPEM,
		hostKeyPath:   hostKeyPEM,
		phoneKeyPath:  phoneKeyPEM,
	} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatalf("write %s: %v", filepath.Base(path), err)
		}
	}

	// Start a gRPC server on the "phone" side using mTLS pinned to host cert.
	lis, err := mtls.ListenMTLS("127.0.0.1:0", phoneCertPath, phoneKeyPath, hostFP, "")
	if err != nil {
		t.Fatalf("ListenMTLS: %v", err)
	}
	defer lis.Close()

	gs := grpc.NewServer()
	nixkeyv1.RegisterNixKeyAgentServer(gs, &mockPhoneServer{})
	go func() { _ = gs.Serve(lis) }()
	defer gs.GracefulStop()

	// Step 1: Verify mTLS works before revocation.
	conn, err := mtls.DialMTLS(lis.Addr().String(), hostCertPath, hostKeyPath, phoneFP, "")
	if err != nil {
		t.Fatalf("pre-revoke DialMTLS: %v", err)
	}

	client := nixkeyv1.NewNixKeyAgentClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err = client.Ping(ctx, &nixkeyv1.PingRequest{})
	if err != nil {
		t.Fatalf("pre-revoke Ping should succeed: %v", err)
	}
	_ = conn.Close()

	// Register device in daemon registry.
	reg.Add(daemon.Device{
		ID:              phoneFP[:16],
		Name:            "test-phone",
		TailscaleIP:     "100.64.0.99",
		ListenPort:      29418,
		CertFingerprint: phoneFP,
		CertPath:        phoneCertPath,
		ClientCertPath:  hostCertPath,
		ClientKeyPath:   hostKeyPath,
		Source:          daemon.SourceRuntimePaired,
	})

	// Step 2: Revoke the device via control socket.
	resp := sendCommand(t, socketPath, daemon.Request{
		Command:  "revoke-device",
		DeviceID: phoneFP[:16],
	})
	if resp.Status != "ok" {
		t.Fatalf("revoke-device: expected ok, got %q (error: %s)", resp.Status, resp.Error)
	}

	// Step 3: Verify device removed from registry.
	if _, ok := reg.Get(phoneFP[:16]); ok {
		t.Fatal("device should have been removed from registry after revocation")
	}

	// Step 4: Verify cert files are deleted.
	if _, err := os.Stat(hostCertPath); err == nil {
		t.Fatal("host client cert should have been deleted")
	}
	if _, err := os.Stat(hostKeyPath); err == nil {
		t.Fatal("host client key should have been deleted")
	}

	// Step 5: Attempting mTLS dial with deleted cert files fails (FR-E09).
	_, err = mtls.DialMTLS(lis.Addr().String(), hostCertPath, hostKeyPath, phoneFP, "")
	if err == nil {
		// grpc.NewClient may not fail immediately, so try an actual RPC.
		t.Fatal("expected DialMTLS to fail after cert files deleted")
	}
	// The error should indicate the cert files are gone.
	t.Logf("FR-E09: post-revoke dial correctly failed: %v", err)
}

type mockPhoneServer struct {
	nixkeyv1.UnimplementedNixKeyAgentServer
}

func (m *mockPhoneServer) Ping(_ context.Context, _ *nixkeyv1.PingRequest) (*nixkeyv1.PingResponse, error) {
	return &nixkeyv1.PingResponse{TimestampMs: time.Now().UnixMilli()}, nil
}
