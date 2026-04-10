package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/phaedrus-raznikov/nix-key/internal/daemon"
)

func startTestControlServerForExport(t *testing.T, keys []daemon.KeyInfo) string {
	t.Helper()

	dir := t.TempDir()
	socketPath := filepath.Join(dir, "control.sock")
	devicesPath := filepath.Join(dir, "devices.json")

	reg := daemon.NewRegistry()

	srv := daemon.NewControlServer(daemon.ControlServerConfig{
		SocketPath:  socketPath,
		Registry:    reg,
		DevicesPath: devicesPath,
		KeyLister:   func() []daemon.KeyInfo { return keys },
	})

	if err := srv.Start(); err != nil {
		t.Fatalf("start control server: %v", err)
	}
	t.Cleanup(func() { srv.Stop() })

	return socketPath
}

func TestRunExportExactFingerprint(t *testing.T) {
	keys := []daemon.KeyInfo{
		{
			Fingerprint: "SHA256:abcdef1234567890",
			KeyType:     "ssh-ed25519",
			DisplayName: "test-key-1",
			DeviceID:    "dev-1",
			PublicKey:    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITest1 test-key-1",
		},
		{
			Fingerprint: "SHA256:xyz9876543210000",
			KeyType:     "ecdsa-sha2-nistp256",
			DisplayName: "test-key-2",
			DeviceID:    "dev-2",
			PublicKey:    "ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTY= test-key-2",
		},
	}

	socketPath := startTestControlServerForExport(t, keys)

	var buf strings.Builder
	err := runExport(socketPath, "SHA256:abcdef1234567890", &buf)
	if err != nil {
		t.Fatalf("runExport: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITest1 test-key-1") {
		t.Errorf("expected SSH public key output, got:\n%s", output)
	}
}

func TestRunExportUniquePrefix(t *testing.T) {
	keys := []daemon.KeyInfo{
		{
			Fingerprint: "SHA256:abcdef1234567890",
			KeyType:     "ssh-ed25519",
			DisplayName: "test-key-1",
			DeviceID:    "dev-1",
			PublicKey:    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITest1 test-key-1",
		},
		{
			Fingerprint: "SHA256:xyz9876543210000",
			KeyType:     "ecdsa-sha2-nistp256",
			DisplayName: "test-key-2",
			DeviceID:    "dev-2",
			PublicKey:    "ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTY= test-key-2",
		},
	}

	socketPath := startTestControlServerForExport(t, keys)

	var buf strings.Builder
	err := runExport(socketPath, "SHA256:abc", &buf)
	if err != nil {
		t.Fatalf("runExport with prefix: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "ssh-ed25519") {
		t.Errorf("expected SSH public key for prefix match, got:\n%s", output)
	}
}

func TestRunExportPrefixWithoutSHA256Prefix(t *testing.T) {
	keys := []daemon.KeyInfo{
		{
			Fingerprint: "SHA256:abcdef1234567890",
			KeyType:     "ssh-ed25519",
			DisplayName: "test-key-1",
			DeviceID:    "dev-1",
			PublicKey:    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITest1 test-key-1",
		},
	}

	socketPath := startTestControlServerForExport(t, keys)

	// User provides just the hash without "SHA256:" prefix.
	var buf strings.Builder
	err := runExport(socketPath, "abcdef", &buf)
	if err != nil {
		t.Fatalf("runExport with bare prefix: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "ssh-ed25519") {
		t.Errorf("expected SSH public key for bare prefix match, got:\n%s", output)
	}
}

func TestRunExportKeyNotFound(t *testing.T) {
	keys := []daemon.KeyInfo{
		{
			Fingerprint: "SHA256:abcdef1234567890",
			KeyType:     "ssh-ed25519",
			DisplayName: "test-key-1",
			DeviceID:    "dev-1",
			PublicKey:    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITest1 test-key-1",
		},
	}

	socketPath := startTestControlServerForExport(t, keys)

	var buf strings.Builder
	err := runExport(socketPath, "SHA256:nonexistent", &buf)
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	if !strings.Contains(err.Error(), "no key found") {
		t.Errorf("error should mention no key found, got: %v", err)
	}
}

func TestRunExportAmbiguousPrefix(t *testing.T) {
	keys := []daemon.KeyInfo{
		{
			Fingerprint: "SHA256:abcdef1234567890",
			KeyType:     "ssh-ed25519",
			DisplayName: "test-key-1",
			DeviceID:    "dev-1",
			PublicKey:    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITest1 test-key-1",
		},
		{
			Fingerprint: "SHA256:abcdef9999999999",
			KeyType:     "ssh-ed25519",
			DisplayName: "test-key-2",
			DeviceID:    "dev-2",
			PublicKey:    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITest2 test-key-2",
		},
	}

	socketPath := startTestControlServerForExport(t, keys)

	var buf strings.Builder
	err := runExport(socketPath, "SHA256:abcdef", &buf)
	if err == nil {
		t.Fatal("expected error for ambiguous prefix")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error should mention ambiguous prefix, got: %v", err)
	}
}

func TestRunExportNoKeys(t *testing.T) {
	socketPath := startTestControlServerForExport(t, []daemon.KeyInfo{})

	var buf strings.Builder
	err := runExport(socketPath, "SHA256:anything", &buf)
	if err == nil {
		t.Fatal("expected error when no keys available")
	}
	if !strings.Contains(err.Error(), "no key found") {
		t.Errorf("error should mention no key found, got: %v", err)
	}
}

func TestRunExportDaemonUnreachable(t *testing.T) {
	var buf strings.Builder
	err := runExport("/tmp/nonexistent-export-socket.sock", "SHA256:abc", &buf)
	if err == nil {
		t.Fatal("expected error when daemon is unreachable")
	}
	if !strings.Contains(err.Error(), "failed to query daemon") {
		t.Errorf("error should mention failed query, got: %v", err)
	}
}
