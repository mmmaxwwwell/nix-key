package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/phaedrus-raznikov/nix-key/internal/daemon"
	"github.com/phaedrus-raznikov/nix-key/internal/mtls"
)

// TestIntegrationColdStartDaemon verifies that the daemon starts successfully
// with no pre-existing state: devices.json is missing, state directory does not
// exist. The daemon should load empty state and SaveToJSON creates dirs with
// 0700 permissions and files with 0600 permissions.
func TestIntegrationColdStartDaemon(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state", "nix-key")

	// Verify state dir does not exist yet.
	if _, err := os.Stat(stateDir); !os.IsNotExist(err) {
		t.Fatal("state dir should not exist before cold start")
	}

	// Load devices from non-existent path — should return nil, nil.
	devicesPath := filepath.Join(stateDir, "devices.json")
	devices, err := daemon.LoadDevicesFromJSON(devicesPath)
	if err != nil {
		t.Fatalf("cold start: LoadDevicesFromJSON should not error for missing file: %v", err)
	}
	if len(devices) != 0 {
		t.Fatalf("cold start: expected 0 devices, got %d", len(devices))
	}

	// Create registry and merge empty sets — simulates daemon startup.
	registry := daemon.NewRegistry()
	registry.Merge(nil, devices)

	if got := registry.ListAll(); len(got) != 0 {
		t.Fatalf("cold start: registry should be empty, got %d devices", len(got))
	}

	// Save triggers state dir creation via SaveToJSON.
	registry.Add(daemon.Device{
		ID:              "cold-start-dev",
		Name:            "Cold Phone",
		TailscaleIP:     "100.64.0.1",
		ListenPort:      29418,
		CertFingerprint: "fp-cold",
		Source:          daemon.SourceRuntimePaired,
	})
	if err := registry.SaveToJSON(devicesPath); err != nil {
		t.Fatalf("cold start: SaveToJSON should create dirs: %v", err)
	}

	// Verify state dir was created with 0700 permissions.
	info, err := os.Stat(stateDir)
	if err != nil {
		t.Fatalf("state dir should exist after SaveToJSON: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("state dir should be a directory")
	}
	if perm := info.Mode().Perm(); perm != 0700 {
		t.Errorf("state dir permissions: got %o, want 0700", perm)
	}

	// Verify devices.json has 0600 permissions.
	fileInfo, err := os.Stat(devicesPath)
	if err != nil {
		t.Fatalf("devices.json should exist: %v", err)
	}
	if perm := fileInfo.Mode().Perm(); perm != 0600 {
		t.Errorf("devices.json permissions: got %o, want 0600", perm)
	}
}

// TestIntegrationColdStartAgeIdentity verifies that age identity generation
// creates parent directories with 0700 and the identity file with 0600.
func TestIntegrationColdStartAgeIdentity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	identityPath := filepath.Join(dir, "deeply", "nested", "state", "age-identity.txt")

	if err := mtls.GenerateIdentity(identityPath); err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}

	// Verify parent directory has 0700.
	parentDir := filepath.Join(dir, "deeply", "nested", "state")
	info, err := os.Stat(parentDir)
	if err != nil {
		t.Fatalf("parent dir should exist: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0700 {
		t.Errorf("identity parent dir permissions: got %o, want 0700", perm)
	}

	// Verify identity file has 0600.
	fileInfo, err := os.Stat(identityPath)
	if err != nil {
		t.Fatalf("identity file should exist: %v", err)
	}
	if perm := fileInfo.Mode().Perm(); perm != 0600 {
		t.Errorf("identity file permissions: got %o, want 0600", perm)
	}
}

// TestIntegrationWarmStartDaemon verifies that the daemon reuses existing state
// on restart: loads devices from devices.json, preserves LastSeen timestamps,
// and re-saves without corruption.
func TestIntegrationWarmStartDaemon(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	devicesPath := filepath.Join(dir, "devices.json")

	// First run: create and persist devices.
	lastSeen := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)
	originalDevices := []daemon.Device{
		{
			ID: "warm-dev-1", Name: "Warm Phone 1",
			TailscaleIP: "100.64.0.10", ListenPort: 29418,
			CertFingerprint: "fp-warm-1",
			CertPath:        "/tmp/cert1.pem",
			ClientCertPath:  "/tmp/client1.pem",
			ClientKeyPath:   "/tmp/client1-key.pem.age",
			LastSeen:        &lastSeen,
			Source:          daemon.SourceRuntimePaired,
		},
		{
			ID: "warm-dev-2", Name: "Warm Phone 2",
			TailscaleIP: "100.64.0.11", ListenPort: 29418,
			CertFingerprint: "fp-warm-2",
			Source:          daemon.SourceRuntimePaired,
		},
	}

	reg1 := daemon.NewRegistry()
	reg1.Merge(nil, originalDevices)
	if err := reg1.SaveToJSON(devicesPath); err != nil {
		t.Fatalf("first run save: %v", err)
	}

	// Warm restart: load from existing devices.json.
	loaded, err := daemon.LoadDevicesFromJSON(devicesPath)
	if err != nil {
		t.Fatalf("warm start load: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("warm start: expected 2 devices, got %d", len(loaded))
	}

	reg2 := daemon.NewRegistry()
	reg2.Merge(nil, loaded)

	all := reg2.ListAll()
	if len(all) != 2 {
		t.Fatalf("warm start registry: expected 2 devices, got %d", len(all))
	}

	// Verify LastSeen preserved.
	dev1, ok := reg2.Get("warm-dev-1")
	if !ok {
		t.Fatal("warm-dev-1 should exist after warm start")
	}
	if dev1.LastSeen == nil || !dev1.LastSeen.Equal(lastSeen) {
		t.Errorf("warm start: LastSeen mismatch, got %v, want %v", dev1.LastSeen, lastSeen)
	}
	if dev1.TailscaleIP != "100.64.0.10" {
		t.Errorf("warm start: IP mismatch, got %q", dev1.TailscaleIP)
	}
	if dev1.ClientKeyPath != "/tmp/client1-key.pem.age" {
		t.Errorf("warm start: ClientKeyPath mismatch, got %q", dev1.ClientKeyPath)
	}

	// Re-save and reload — no corruption.
	if err := reg2.SaveToJSON(devicesPath); err != nil {
		t.Fatalf("warm start re-save: %v", err)
	}
	reloaded, err := daemon.LoadDevicesFromJSON(devicesPath)
	if err != nil {
		t.Fatalf("warm start reload: %v", err)
	}
	if len(reloaded) != 2 {
		t.Fatalf("expected 2 devices after re-save, got %d", len(reloaded))
	}
}

// TestIntegrationWarmStartPreservesDeviceJSON verifies the stop→start cycle
// produces valid JSON with all fields preserved.
func TestIntegrationWarmStartPreservesDeviceJSON(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	devicesPath := filepath.Join(dir, "devices.json")
	now := time.Date(2026, 3, 29, 15, 0, 0, 0, time.UTC)

	reg1 := daemon.NewRegistry()
	reg1.Add(daemon.Device{
		ID: "dev-a", Name: "Phone A", TailscaleIP: "100.64.0.1",
		ListenPort: 29418, CertFingerprint: "fp-a",
		CertPath: "/state/certs/a/server.pem", ClientCertPath: "/state/certs/a/client.pem",
		ClientKeyPath: "/state/certs/a/client-key.pem.age",
		LastSeen: &now, Source: daemon.SourceRuntimePaired,
	})
	reg1.Add(daemon.Device{
		ID: "dev-b", Name: "Phone B", TailscaleIP: "100.64.0.2",
		ListenPort: 29418, CertFingerprint: "fp-b",
		Source: daemon.SourceRuntimePaired,
	})
	if err := reg1.SaveToJSON(devicesPath); err != nil {
		t.Fatal(err)
	}

	// Verify raw JSON is valid.
	rawJSON, err := os.ReadFile(devicesPath)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(rawJSON) {
		t.Fatal("devices.json should be valid JSON")
	}

	// Reload and verify all fields.
	loaded, err := daemon.LoadDevicesFromJSON(devicesPath)
	if err != nil {
		t.Fatal(err)
	}
	reg2 := daemon.NewRegistry()
	reg2.Merge(nil, loaded)

	devA, ok := reg2.Get("dev-a")
	if !ok {
		t.Fatal("dev-a should exist after warm start")
	}
	if devA.ClientKeyPath != "/state/certs/a/client-key.pem.age" {
		t.Errorf("ClientKeyPath not preserved: %q", devA.ClientKeyPath)
	}
	if devA.LastSeen == nil || !devA.LastSeen.Equal(now) {
		t.Error("LastSeen not preserved across restart")
	}

	devB, ok := reg2.Get("dev-b")
	if !ok {
		t.Fatal("dev-b should exist after warm start")
	}
	if devB.TailscaleIP != "100.64.0.2" {
		t.Errorf("TailscaleIP not preserved: %q", devB.TailscaleIP)
	}
}

// TestIntegrationColdStartControlServer verifies that the control server starts
// on a fresh socket directory.
func TestIntegrationColdStartControlServer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Use /tmp with a short prefix to keep the Unix socket path under the
	// 108-character sun_path limit. CI runners have long TMPDIR paths that
	// cause t.TempDir() + test name + subdirs to exceed the limit.
	dir, err := os.MkdirTemp("/tmp", "nk-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	socketDir := filepath.Join(dir, "run")
	socketPath := filepath.Join(socketDir, "ctl.sock")

	if _, err := os.Stat(socketDir); !os.IsNotExist(err) {
		t.Fatal("socket dir should not exist yet")
	}

	if err := os.MkdirAll(socketDir, 0700); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(socketDir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0700 {
		t.Errorf("socket dir permissions: got %o, want 0700", perm)
	}

	reg := daemon.NewRegistry()
	srv := daemon.NewControlServer(daemon.ControlServerConfig{
		SocketPath:  socketPath,
		Registry:    reg,
		DevicesPath: filepath.Join(dir, "devices.json"),
		KeyLister:   func() []daemon.KeyInfo { return nil },
	})
	if err := srv.Start(); err != nil {
		t.Fatalf("control server start: %v", err)
	}
	t.Cleanup(func() { srv.Stop() })

	if _, err := os.Stat(socketPath); err != nil {
		t.Fatalf("control socket should exist: %v", err)
	}

	client := daemon.NewControlClient(socketPath)
	resp, err := client.SendCommand(daemon.Request{Command: "get-status"})
	if err != nil {
		t.Fatalf("get-status command: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("get-status response: got %q, want ok", resp.Status)
	}
}

// TestIntegrationSecretsAtRestAgeKeyFileOption verifies that the ageKeyFile
// config option correctly decrypts age-encrypted keys, and that decryption
// with the wrong identity fails.
func TestIntegrationSecretsAtRestAgeKeyFileOption(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()

	// Generate age identity at a custom path.
	customAgeKeyFile := filepath.Join(dir, "custom", "my-age-key.txt")
	if err := mtls.GenerateIdentity(customAgeKeyFile); err != nil {
		t.Fatalf("generate custom age identity: %v", err)
	}

	// Create a plaintext payload, encrypt it with age.
	plainKey := []byte("test-secret-key-material-for-age-encryption-verification")
	plainPath := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(plainPath, plainKey, 0600); err != nil {
		t.Fatal(err)
	}
	if err := mtls.EncryptFile(plainPath, customAgeKeyFile); err != nil {
		t.Fatalf("EncryptFile: %v", err)
	}

	encPath := plainPath + ".age"

	// Decrypt works with correct key.
	decrypted, err := mtls.DecryptToMemory(encPath, customAgeKeyFile)
	if err != nil {
		t.Fatalf("DecryptToMemory with custom ageKeyFile: %v", err)
	}
	if string(decrypted) != string(plainKey) {
		t.Error("decrypted data should match original plaintext key")
	}

	// Decrypt fails with wrong identity.
	wrongKeyFile := filepath.Join(dir, "wrong-key.txt")
	if err := mtls.GenerateIdentity(wrongKeyFile); err != nil {
		t.Fatal(err)
	}
	_, err = mtls.DecryptToMemory(encPath, wrongKeyFile)
	if err == nil {
		t.Error("decryption with wrong ageKeyFile should fail")
	}
}
