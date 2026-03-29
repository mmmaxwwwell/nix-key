package daemon_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/phaedrus-raznikov/nix-key/internal/daemon"
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
