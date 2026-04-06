package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/phaedrus-raznikov/nix-key/internal/daemon"
)

func startTestControlServerForRevoke(t *testing.T) (*daemon.Registry, string) {
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

func TestRunRevokeSuccess(t *testing.T) {
	reg, socketPath := startTestControlServerForRevoke(t)

	reg.Add(daemon.Device{
		ID:              "dev-to-revoke",
		Name:            "Test Phone",
		TailscaleIP:     "100.64.0.2",
		ListenPort:      29418,
		CertFingerprint: "abc123",
		Source:          daemon.SourceRuntimePaired,
	})

	var buf strings.Builder
	err := runRevoke(socketPath, "dev-to-revoke", &buf)
	if err != nil {
		t.Fatalf("runRevoke: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "dev-to-revoke") {
		t.Errorf("output should contain device ID, got: %s", output)
	}
	if !strings.Contains(output, "revoked successfully") {
		t.Errorf("output should confirm revocation, got: %s", output)
	}

	// Verify device was actually removed from registry.
	if _, ok := reg.Get("dev-to-revoke"); ok {
		t.Error("device should have been removed from registry")
	}
}

func TestRunRevokeNixDeclaredDevice(t *testing.T) {
	reg, socketPath := startTestControlServerForRevoke(t)

	reg.Add(daemon.Device{
		ID:              "nix-device",
		Name:            "Nix Phone",
		TailscaleIP:     "100.64.0.3",
		ListenPort:      29418,
		CertFingerprint: "nix-fp-123",
		Source:          daemon.SourceNixDeclared,
	})

	var buf strings.Builder
	err := runRevoke(socketPath, "nix-device", &buf)
	if err == nil {
		t.Fatal("expected error for nix-declared device")
	}

	if !strings.Contains(err.Error(), "NixOS") || !strings.Contains(err.Error(), "nix-declared") {
		t.Errorf("error should mention NixOS configuration, got: %v", err)
	}

	// Verify device was NOT removed.
	if _, ok := reg.Get("nix-device"); !ok {
		t.Error("nix-declared device should NOT have been removed")
	}
}

func TestRunRevokeNonexistentDevice(t *testing.T) {
	_, socketPath := startTestControlServerForRevoke(t)

	var buf strings.Builder
	err := runRevoke(socketPath, "does-not-exist", &buf)
	if err == nil {
		t.Fatal("expected error for nonexistent device")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention device not found, got: %v", err)
	}
}

func TestRunRevokeDaemonUnreachable(t *testing.T) {
	var buf strings.Builder
	err := runRevoke("/tmp/nonexistent-socket-path.sock", "some-device", &buf)
	if err == nil {
		t.Fatal("expected error when daemon is unreachable")
	}

	if !strings.Contains(err.Error(), "failed to query daemon") {
		t.Errorf("error should mention daemon connection failure, got: %v", err)
	}
}
