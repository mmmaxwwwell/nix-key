package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/phaedrus-raznikov/nix-key/internal/config"
	"github.com/phaedrus-raznikov/nix-key/internal/daemon"
	"github.com/phaedrus-raznikov/nix-key/internal/mtls"
)

// startIntegrationDaemon creates a shared control server with test fixtures
// for integration tests. Returns registry, socketPath, and a temp dir for files.
func startIntegrationDaemon(t *testing.T, keys []daemon.KeyInfo) (*daemon.Registry, string, string) {
	t.Helper()

	dir := t.TempDir()
	socketPath := filepath.Join(dir, "control.sock")
	devicesPath := filepath.Join(dir, "devices.json")

	reg := daemon.NewRegistry()

	if keys == nil {
		keys = []daemon.KeyInfo{}
	}
	keysCopy := keys

	srv := daemon.NewControlServer(daemon.ControlServerConfig{
		SocketPath:  socketPath,
		Registry:    reg,
		DevicesPath: devicesPath,
		KeyLister:   func() []daemon.KeyInfo { return keysCopy },
	})

	if err := srv.Start(); err != nil {
		t.Fatalf("start control server: %v", err)
	}
	t.Cleanup(func() { srv.Stop() })

	return reg, socketPath, dir
}

// TestIntegrationDevicesListAndFormat starts a daemon with multiple devices
// and verifies the devices command output format and content.
func TestIntegrationDevicesListAndFormat(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	reg, socketPath, _ := startIntegrationDaemon(t, nil)

	now := time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)
	reg.Add(daemon.Device{
		ID:              "pixel-7",
		Name:            "Pixel 7",
		TailscaleIP:     "100.64.0.1",
		ListenPort:      29418,
		CertFingerprint: "SHA256:aabbccdd11223344",
		LastSeen:        &now,
		Source:          daemon.SourceRuntimePaired,
	})
	reg.Add(daemon.Device{
		ID:              "samsung-s24",
		Name:            "Samsung S24",
		TailscaleIP:     "100.64.0.2",
		ListenPort:      29418,
		CertFingerprint: "SHA256:eeff00112233aabb",
		Source:          daemon.SourceNixDeclared,
	})

	// Query via control client and verify table output.
	client := daemon.NewControlClient(socketPath)
	resp, err := client.SendCommand(daemon.Request{Command: "list-devices"})
	if err != nil {
		t.Fatalf("list-devices: %v", err)
	}

	devices, err := parseDeviceInfos(resp)
	if err != nil {
		t.Fatalf("parseDeviceInfos: %v", err)
	}

	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}

	var buf strings.Builder
	formatDevicesTable(&buf, devices)
	output := buf.String()

	for _, want := range []string{
		"NAME", "TAILSCALE IP", "CERT FINGERPRINT", "LAST SEEN", "SOURCE",
		"Pixel 7", "100.64.0.1", "runtime-paired",
		"Samsung S24", "100.64.0.2", "nix-declared",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("devices output missing %q:\n%s", want, output)
		}
	}
}

// TestIntegrationStatusWithDevicesAndKeys verifies the status command reports
// correct device count, key count, and running state.
func TestIntegrationStatusWithDevicesAndKeys(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	keys := []daemon.KeyInfo{
		{Fingerprint: "SHA256:key1aaa", KeyType: "ssh-ed25519", DisplayName: "key1", DeviceID: "dev-1"},
		{Fingerprint: "SHA256:key2bbb", KeyType: "ecdsa-sha2-nistp256", DisplayName: "key2", DeviceID: "dev-1"},
		{Fingerprint: "SHA256:key3ccc", KeyType: "ssh-ed25519", DisplayName: "key3", DeviceID: "dev-2"},
	}

	reg, socketPath, _ := startIntegrationDaemon(t, keys)

	reg.Add(daemon.Device{
		ID: "dev-1", Name: "Phone A", TailscaleIP: "100.64.0.1",
		ListenPort: 29418, CertFingerprint: "fp-1", Source: daemon.SourceRuntimePaired,
	})
	reg.Add(daemon.Device{
		ID: "dev-2", Name: "Phone B", TailscaleIP: "100.64.0.2",
		ListenPort: 29418, CertFingerprint: "fp-2", Source: daemon.SourceNixDeclared,
	})

	var buf strings.Builder
	err := runStatus(socketPath, &buf)
	if err != nil {
		t.Fatalf("runStatus: %v", err)
	}

	output := buf.String()
	for _, want := range []string{"running", "2", "3"} {
		if !strings.Contains(output, want) {
			t.Errorf("status output missing %q:\n%s", want, output)
		}
	}
}

// TestIntegrationExportKey verifies export finds a key by fingerprint and
// prints the public key in SSH format.
func TestIntegrationExportKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	keys := []daemon.KeyInfo{
		{
			Fingerprint: "SHA256:abcdef1234567890",
			KeyType:     "ssh-ed25519",
			DisplayName: "test-key",
			DeviceID:    "dev-1",
			PublicKey:   "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITest test-key",
		},
	}

	_, socketPath, _ := startIntegrationDaemon(t, keys)

	var buf strings.Builder
	err := runExport(socketPath, "SHA256:abcdef1234567890", &buf)
	if err != nil {
		t.Fatalf("runExport: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITest test-key") {
		t.Errorf("export output missing public key:\n%s", output)
	}
}

// TestIntegrationRevokeAndVerifyRemoval verifies that revoking a device removes
// it from the registry, and a subsequent list-devices no longer includes it.
func TestIntegrationRevokeAndVerifyRemoval(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	reg, socketPath, _ := startIntegrationDaemon(t, nil)

	reg.Add(daemon.Device{
		ID: "dev-to-revoke", Name: "Revoke Me", TailscaleIP: "100.64.0.5",
		ListenPort: 29418, CertFingerprint: "fp-revoke", Source: daemon.SourceRuntimePaired,
	})
	reg.Add(daemon.Device{
		ID: "dev-keep", Name: "Keep Me", TailscaleIP: "100.64.0.6",
		ListenPort: 29418, CertFingerprint: "fp-keep", Source: daemon.SourceRuntimePaired,
	})

	// Revoke.
	var buf strings.Builder
	err := runRevoke(socketPath, "dev-to-revoke", &buf)
	if err != nil {
		t.Fatalf("runRevoke: %v", err)
	}
	if !strings.Contains(buf.String(), "revoked successfully") {
		t.Errorf("revoke output missing confirmation:\n%s", buf.String())
	}

	// Verify device removed from registry.
	if _, ok := reg.Get("dev-to-revoke"); ok {
		t.Error("revoked device still present in registry")
	}

	// Verify remaining device still exists.
	if _, ok := reg.Get("dev-keep"); !ok {
		t.Error("unrelated device was removed after revoke")
	}

	// Verify list-devices no longer includes revoked device.
	client := daemon.NewControlClient(socketPath)
	resp, err := client.SendCommand(daemon.Request{Command: "list-devices"})
	if err != nil {
		t.Fatalf("list-devices after revoke: %v", err)
	}
	devices, err := parseDeviceInfos(resp)
	if err != nil {
		t.Fatalf("parseDeviceInfos: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device after revoke, got %d", len(devices))
	}
	if devices[0].ID != "dev-keep" {
		t.Errorf("expected remaining device 'dev-keep', got %q", devices[0].ID)
	}
}

// TestIntegrationRevokeCleansUpCertFiles verifies that revoking a device
// deletes its certificate files from disk.
func TestIntegrationRevokeCleansUpCertFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	reg, socketPath, dir := startIntegrationDaemon(t, nil)

	// Create cert files on disk.
	certDir := filepath.Join(dir, "certs", "dev-cert-test")
	if err := os.MkdirAll(certDir, 0700); err != nil {
		t.Fatal(err)
	}

	certPath := filepath.Join(certDir, "server.crt")
	clientCertPath := filepath.Join(certDir, "client.crt")
	clientKeyPath := filepath.Join(certDir, "client.key")

	for _, p := range []string{certPath, clientCertPath, clientKeyPath} {
		if err := os.WriteFile(p, []byte("placeholder"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	reg.Add(daemon.Device{
		ID: "dev-cert-cleanup", Name: "Cert Test", TailscaleIP: "100.64.0.7",
		ListenPort: 29418, CertFingerprint: "fp-cert",
		CertPath: certPath, ClientCertPath: clientCertPath, ClientKeyPath: clientKeyPath,
		Source: daemon.SourceRuntimePaired,
	})

	var buf strings.Builder
	err := runRevoke(socketPath, "dev-cert-cleanup", &buf)
	if err != nil {
		t.Fatalf("runRevoke: %v", err)
	}

	// Verify cert files deleted.
	for _, p := range []string{certPath, clientCertPath, clientKeyPath} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("cert file %s should have been deleted", p)
		}
	}
}

// TestIntegrationStatusWithCertWarning verifies that status reports cert
// expiry warnings when a device's cert is close to expiration.
func TestIntegrationStatusWithCertWarning(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	reg, socketPath, dir := startIntegrationDaemon(t, nil)

	// Generate a cert that expires in 10 days.
	certPEM, _, err := mtls.GenerateCert(mtls.CertOptions{
		KeyType:    mtls.KeyTypeEd25519,
		CommonName: "expiring-phone",
		Expiry:     10 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("generate cert: %v", err)
	}

	certPath := filepath.Join(dir, "expiring-cert.pem")
	if err := os.WriteFile(certPath, certPEM, 0600); err != nil {
		t.Fatal(err)
	}

	reg.Add(daemon.Device{
		ID: "dev-expiring", Name: "Expiring Phone", TailscaleIP: "100.64.0.10",
		ListenPort: 29418, CertFingerprint: "fp-exp", CertPath: certPath,
		Source: daemon.SourceRuntimePaired,
	})

	var buf strings.Builder
	err = runStatus(socketPath, &buf)
	if err != nil {
		t.Fatalf("runStatus: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Certificate warnings") {
		t.Errorf("status should show cert warnings:\n%s", output)
	}
	if !strings.Contains(output, "Expiring Phone") {
		t.Errorf("status should mention device name:\n%s", output)
	}
}

// TestIntegrationConfigDisplay verifies config reads and displays config file
// with sensitive fields masked.
func TestIntegrationConfigDisplay(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	content := `{
  "port": 29418,
  "tailscaleInterface": "tailscale0",
  "allowKeyListing": true,
  "signTimeout": 30,
  "connectionTimeout": 10,
  "socketPath": "/run/user/1000/nix-key/agent.sock",
  "controlSocketPath": "/run/user/1000/nix-key/control.sock",
  "logLevel": "info",
  "ageKeyFile": "/home/user/.local/state/nix-key/age-identity.txt",
  "tailscaleAuthKeyFile": "/etc/nix-key/ts-auth-key",
  "certExpiry": "365d"
}`
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	var buf strings.Builder
	err := runConfig(configPath, &buf)
	if err != nil {
		t.Fatalf("runConfig: %v", err)
	}

	output := buf.String()

	// Non-sensitive values displayed.
	for _, want := range []string{"29418", "tailscale0", "info"} {
		if !strings.Contains(output, want) {
			t.Errorf("config output missing %q:\n%s", want, output)
		}
	}

	// Sensitive values masked.
	if strings.Contains(output, "/home/user/.local/state/nix-key/age-identity.txt") {
		t.Errorf("config should mask ageKeyFile:\n%s", output)
	}
	if strings.Contains(output, "/etc/nix-key/ts-auth-key") {
		t.Errorf("config should mask tailscaleAuthKeyFile:\n%s", output)
	}
	if !strings.Contains(output, "present") {
		t.Errorf("config should show 'present' for sensitive fields:\n%s", output)
	}
}

// --- Error case tests ---

// TestIntegrationRevokeNonexistentDevice verifies revoke returns a clear error
// when the device ID does not exist.
func TestIntegrationRevokeNonexistentDevice(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, socketPath, _ := startIntegrationDaemon(t, nil)

	var buf strings.Builder
	err := runRevoke(socketPath, "nonexistent-device-id", &buf)
	if err == nil {
		t.Fatal("expected error for nonexistent device")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", err)
	}
}

// TestIntegrationRevokeNixDeclaredDevice verifies revoke rejects nix-declared
// devices with a helpful error message.
func TestIntegrationRevokeNixDeclaredDevice(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	reg, socketPath, _ := startIntegrationDaemon(t, nil)

	reg.Add(daemon.Device{
		ID: "nix-dev", Name: "Nix Phone", TailscaleIP: "100.64.0.3",
		ListenPort: 29418, CertFingerprint: "nix-fp",
		Source: daemon.SourceNixDeclared,
	})

	var buf strings.Builder
	err := runRevoke(socketPath, "nix-dev", &buf)
	if err == nil {
		t.Fatal("expected error for nix-declared device")
	}
	if !strings.Contains(err.Error(), "nix-declared") || !strings.Contains(err.Error(), "NixOS") {
		t.Errorf("error should mention NixOS config, got: %v", err)
	}

	// Device should still be in registry.
	if _, ok := reg.Get("nix-dev"); !ok {
		t.Error("nix-declared device should not have been removed")
	}
}

// TestIntegrationExportUnknownKey verifies export returns a clear error
// when no key matches the given fingerprint.
func TestIntegrationExportUnknownKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	keys := []daemon.KeyInfo{
		{Fingerprint: "SHA256:realkey111", KeyType: "ssh-ed25519", DisplayName: "key1", DeviceID: "d1",
			PublicKey: "ssh-ed25519 AAAA key1"},
	}
	_, socketPath, _ := startIntegrationDaemon(t, keys)

	var buf strings.Builder
	err := runExport(socketPath, "SHA256:doesnotexist", &buf)
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	if !strings.Contains(err.Error(), "no key found") {
		t.Errorf("error should mention 'no key found', got: %v", err)
	}
}

// TestIntegrationExportAmbiguousPrefix verifies export returns a clear error
// when multiple keys match the given prefix.
func TestIntegrationExportAmbiguousPrefix(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	keys := []daemon.KeyInfo{
		{Fingerprint: "SHA256:abcdef111", KeyType: "ssh-ed25519", DisplayName: "k1", DeviceID: "d1",
			PublicKey: "ssh-ed25519 AAAA1 k1"},
		{Fingerprint: "SHA256:abcdef222", KeyType: "ssh-ed25519", DisplayName: "k2", DeviceID: "d2",
			PublicKey: "ssh-ed25519 AAAA2 k2"},
	}
	_, socketPath, _ := startIntegrationDaemon(t, keys)

	var buf strings.Builder
	err := runExport(socketPath, "SHA256:abcdef", &buf)
	if err == nil {
		t.Fatal("expected error for ambiguous prefix")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error should mention 'ambiguous', got: %v", err)
	}
}

// TestIntegrationTestDeviceUnreachable verifies the test command reports a
// clear error for a device with no Tailscale IP.
func TestIntegrationTestDeviceUnreachable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	reg, socketPath, _ := startIntegrationDaemon(t, nil)

	reg.Add(daemon.Device{
		ID: "dev-no-ip", Name: "No IP Phone",
		CertFingerprint: "fp-noip",
		Source:          daemon.SourceRuntimePaired,
	})

	var buf strings.Builder
	err := runTestDevice(socketPath, "dev-no-ip", "", 0, &buf)
	if err == nil {
		t.Fatal("expected error for device without IP")
	}
	if !strings.Contains(err.Error(), "unreachable") {
		t.Errorf("error should mention 'unreachable', got: %v", err)
	}
}

// TestIntegrationTestDeviceNonexistent verifies the test command reports a
// clear error when the device does not exist.
func TestIntegrationTestDeviceNonexistent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, socketPath, _ := startIntegrationDaemon(t, nil)

	var buf strings.Builder
	err := runTestDevice(socketPath, "ghost-device", "", 0, &buf)
	if err == nil {
		t.Fatal("expected error for nonexistent device")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", err)
	}
}

// TestIntegrationTestDeviceMissingCerts verifies the test command reports an
// error when the device has an IP but no cert files configured.
func TestIntegrationTestDeviceMissingCerts(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	reg, socketPath, _ := startIntegrationDaemon(t, nil)

	reg.Add(daemon.Device{
		ID: "dev-no-certs", Name: "No Certs Phone",
		TailscaleIP: "100.64.0.9", ListenPort: 29418,
		CertFingerprint: "fp-nocerts",
		Source:          daemon.SourceRuntimePaired,
	})

	var buf strings.Builder
	err := runTestDevice(socketPath, "dev-no-certs", "", 0, &buf)
	if err == nil {
		t.Fatal("expected error for device without certs")
	}
	if !strings.Contains(err.Error(), "cert") {
		t.Errorf("error should mention cert issue, got: %v", err)
	}
}

// TestIntegrationDaemonUnreachableErrors verifies all commands that require
// the daemon return clear errors when the control socket is unavailable.
func TestIntegrationDaemonUnreachableErrors(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	deadSocket := "/tmp/nonexistent-integration-test-socket.sock"

	t.Run("devices", func(t *testing.T) {
		err := runDevices(deadSocket)
		if err == nil {
			t.Fatal("expected error when daemon unreachable")
		}
		if !strings.Contains(err.Error(), "failed to query daemon") {
			t.Errorf("error should mention query failure, got: %v", err)
		}
	})

	t.Run("status", func(t *testing.T) {
		var buf strings.Builder
		err := runStatus(deadSocket, &buf)
		if err == nil {
			t.Fatal("expected error when daemon unreachable")
		}
		if !strings.Contains(err.Error(), "not running") {
			t.Errorf("error should mention not running, got: %v", err)
		}
	})

	t.Run("revoke", func(t *testing.T) {
		var buf strings.Builder
		err := runRevoke(deadSocket, "some-device", &buf)
		if err == nil {
			t.Fatal("expected error when daemon unreachable")
		}
		if !strings.Contains(err.Error(), "failed to query daemon") {
			t.Errorf("error should mention query failure, got: %v", err)
		}
	})

	t.Run("export", func(t *testing.T) {
		var buf strings.Builder
		err := runExport(deadSocket, "SHA256:abc", &buf)
		if err == nil {
			t.Fatal("expected error when daemon unreachable")
		}
		if !strings.Contains(err.Error(), "failed to query daemon") {
			t.Errorf("error should mention query failure, got: %v", err)
		}
	})

	t.Run("test-device", func(t *testing.T) {
		var buf strings.Builder
		err := runTestDevice(deadSocket, "some-device", "", 0, &buf)
		if err == nil {
			t.Fatal("expected error when daemon unreachable")
		}
		if !strings.Contains(err.Error(), "failed to query daemon") {
			t.Errorf("error should mention query failure, got: %v", err)
		}
	})
}

// TestIntegrationFullWorkflow tests a realistic sequence: add devices, list them,
// check status, export a key, revoke a device, verify post-revocation state.
func TestIntegrationFullWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	keys := []daemon.KeyInfo{
		{
			Fingerprint: "SHA256:workflow-key-1",
			KeyType:     "ssh-ed25519",
			DisplayName: "workflow-key",
			DeviceID:    "phone-a",
			PublicKey:   "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIWF workflow-key",
		},
	}

	reg, socketPath, _ := startIntegrationDaemon(t, keys)

	now := time.Date(2026, 3, 29, 14, 0, 0, 0, time.UTC)
	reg.Add(daemon.Device{
		ID: "phone-a", Name: "Phone A", TailscaleIP: "100.64.0.1",
		ListenPort: 29418, CertFingerprint: "fp-a",
		LastSeen: &now, Source: daemon.SourceRuntimePaired,
	})
	reg.Add(daemon.Device{
		ID: "phone-b", Name: "Phone B", TailscaleIP: "100.64.0.2",
		ListenPort: 29418, CertFingerprint: "fp-b",
		Source: daemon.SourceRuntimePaired,
	})

	// Step 1: List devices — should see both.
	client := daemon.NewControlClient(socketPath)
	resp, err := client.SendCommand(daemon.Request{Command: "list-devices"})
	if err != nil {
		t.Fatalf("list-devices: %v", err)
	}
	devices, err := parseDeviceInfos(resp)
	if err != nil {
		t.Fatalf("parseDeviceInfos: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}

	// Step 2: Check status — should report 2 devices, 1 key.
	var statusBuf strings.Builder
	if err := runStatus(socketPath, &statusBuf); err != nil {
		t.Fatalf("runStatus: %v", err)
	}
	statusOut := statusBuf.String()
	if !strings.Contains(statusOut, "2") {
		t.Errorf("status should show 2 devices:\n%s", statusOut)
	}
	if !strings.Contains(statusOut, "1") {
		t.Errorf("status should show 1 key:\n%s", statusOut)
	}

	// Step 3: Export key.
	var exportBuf strings.Builder
	if err := runExport(socketPath, "SHA256:workflow-key-1", &exportBuf); err != nil {
		t.Fatalf("runExport: %v", err)
	}
	if !strings.Contains(exportBuf.String(), "ssh-ed25519") {
		t.Errorf("export should contain public key:\n%s", exportBuf.String())
	}

	// Step 4: Revoke phone-b.
	var revokeBuf strings.Builder
	if err := runRevoke(socketPath, "phone-b", &revokeBuf); err != nil {
		t.Fatalf("runRevoke: %v", err)
	}

	// Step 5: Verify post-revocation — only phone-a remains.
	resp, err = client.SendCommand(daemon.Request{Command: "list-devices"})
	if err != nil {
		t.Fatalf("list-devices after revoke: %v", err)
	}
	devices, err = parseDeviceInfos(resp)
	if err != nil {
		t.Fatalf("parseDeviceInfos after revoke: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device after revoke, got %d", len(devices))
	}
	if devices[0].Name != "Phone A" {
		t.Errorf("remaining device should be 'Phone A', got %q", devices[0].Name)
	}

	// Step 6: Status should now show 1 device.
	var statusBuf2 strings.Builder
	if err := runStatus(socketPath, &statusBuf2); err != nil {
		t.Fatalf("runStatus after revoke: %v", err)
	}
	statusOut2 := statusBuf2.String()
	if !strings.Contains(statusOut2, "1") {
		t.Errorf("status should show 1 device after revoke:\n%s", statusOut2)
	}
}

// TestIntegrationConfigMissingFile verifies config returns error for missing file.
func TestIntegrationConfigMissingFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	var buf strings.Builder
	err := runConfig("/tmp/nonexistent-integration-config-12345.json", &buf)
	if err == nil {
		t.Fatal("expected error for missing config file")
	}
}

// TestIntegrationLogFormatting verifies log formatting handles JSON and
// non-JSON lines correctly.
func TestIntegrationLogFormatting(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	input := strings.NewReader(strings.Join([]string{
		`{"timestamp":"2026-03-29T10:00:00Z","level":"INFO","msg":"daemon started","module":"agent"}`,
		`{"timestamp":"2026-03-29T10:00:01Z","level":"ERROR","msg":"connection failed","module":"mtls","peer":"192.168.1.5"}`,
		`-- Journal begins at Mon 2026-03-29 09:00:00 UTC. --`,
		`{"timestamp":"2026-03-29T10:00:02Z","level":"WARN","msg":"cert expiring soon"}`,
	}, "\n"))

	var buf strings.Builder
	err := formatLogStream(input, &buf, true)
	if err != nil {
		t.Fatalf("formatLogStream: %v", err)
	}

	output := buf.String()
	for _, want := range []string{
		"daemon started", "module=agent",
		"connection failed", "peer=192.168.1.5",
		"-- Journal begins at",
		"cert expiring soon",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("log output missing %q:\n%s", want, output)
		}
	}
}

// TestIntegrationDevicesEmptyList verifies devices output when no devices exist.
func TestIntegrationDevicesEmptyList(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, socketPath, _ := startIntegrationDaemon(t, nil)

	client := daemon.NewControlClient(socketPath)
	resp, err := client.SendCommand(daemon.Request{Command: "list-devices"})
	if err != nil {
		t.Fatalf("list-devices: %v", err)
	}

	devices, err := parseDeviceInfos(resp)
	if err != nil {
		t.Fatalf("parseDeviceInfos: %v", err)
	}

	var buf strings.Builder
	formatDevicesTable(&buf, devices)
	output := buf.String()

	if !strings.Contains(output, "No devices paired") {
		t.Errorf("empty devices list should show 'No devices paired':\n%s", output)
	}
}

// TestIntegrationNixDevicesAndRuntimeMerge verifies that a daemon with
// Nix-declared devices in config AND runtime-paired devices in devices.json
// produces a registry containing both sources.
func TestIntegrationNixDevicesAndRuntimeMerge(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()

	// Write a config.json with Nix-declared devices.
	cfgData := map[string]interface{}{
		"port":               29418,
		"socketPath":         filepath.Join(dir, "agent.sock"),
		"controlSocketPath":  filepath.Join(dir, "control.sock"),
		"tailscaleInterface": "tailscale0",
		"logLevel":           "info",
		"signTimeout":        30,
		"connectionTimeout":  10,
		"ageKeyFile":         filepath.Join(dir, "age.txt"),
		"certExpiry":         "365d",
		"devices": map[string]interface{}{
			"nix-phone": map[string]interface{}{
				"name":            "nix-phone",
				"tailscaleIp":     "100.64.0.10",
				"port":            50051,
				"certFingerprint": "SHA256:nixfp",
				"clientCertPath":  nil,
				"clientKeyPath":   nil,
			},
		},
	}
	cfgJSON, err := json.Marshal(cfgData)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, cfgJSON, 0600); err != nil {
		t.Fatal(err)
	}

	// Load config and convert Nix devices.
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load config: %v", err)
	}

	var nixDevices []daemon.Device
	for id, dc := range cfg.Devices {
		dev := daemon.Device{
			ID:              id,
			Name:            dc.Name,
			TailscaleIP:     dc.TailscaleIP,
			ListenPort:      dc.Port,
			CertFingerprint: dc.CertFingerprint,
		}
		if dc.ClientCertPath != nil {
			dev.ClientCertPath = *dc.ClientCertPath
		}
		if dc.ClientKeyPath != nil {
			dev.ClientKeyPath = *dc.ClientKeyPath
		}
		nixDevices = append(nixDevices, dev)
	}

	// Write a devices.json with a runtime-paired device.
	devicesPath := filepath.Join(dir, "devices.json")
	runtimeDevs := []daemon.Device{
		{
			ID: "runtime-phone", Name: "Runtime Phone", TailscaleIP: "100.64.0.20",
			ListenPort: 29418, CertFingerprint: "SHA256:rtfp",
			Source: daemon.SourceRuntimePaired,
		},
	}
	devJSON, err := json.Marshal(runtimeDevs)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(devicesPath, devJSON, 0600); err != nil {
		t.Fatal(err)
	}

	runtimeDevices, err := daemon.LoadDevicesFromJSON(devicesPath)
	if err != nil {
		t.Fatalf("LoadDevicesFromJSON: %v", err)
	}

	// Merge both sources into registry.
	registry := daemon.NewRegistry()
	registry.Merge(nixDevices, runtimeDevices)

	// Verify both devices are visible.
	all := registry.ListAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 devices in registry, got %d", len(all))
	}

	nixDev, ok := registry.Get("nix-phone")
	if !ok {
		t.Fatal("nix-phone not found in registry")
	}
	if nixDev.Source != daemon.SourceNixDeclared {
		t.Errorf("nix-phone Source = %q, want %q", nixDev.Source, daemon.SourceNixDeclared)
	}
	if nixDev.TailscaleIP != "100.64.0.10" {
		t.Errorf("nix-phone TailscaleIP = %q, want %q", nixDev.TailscaleIP, "100.64.0.10")
	}
	if nixDev.ListenPort != 50051 {
		t.Errorf("nix-phone ListenPort = %d, want 50051", nixDev.ListenPort)
	}

	rtDev, ok := registry.Get("runtime-phone")
	if !ok {
		t.Fatal("runtime-phone not found in registry")
	}
	if rtDev.Source != daemon.SourceRuntimePaired {
		t.Errorf("runtime-phone Source = %q, want %q", rtDev.Source, daemon.SourceRuntimePaired)
	}
	if rtDev.TailscaleIP != "100.64.0.20" {
		t.Errorf("runtime-phone TailscaleIP = %q, want %q", rtDev.TailscaleIP, "100.64.0.20")
	}
}
