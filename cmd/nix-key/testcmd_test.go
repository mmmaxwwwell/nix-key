package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/phaedrus-raznikov/nix-key/internal/daemon"
)

func startTestControlServerForTest(t *testing.T) (*daemon.Registry, string) {
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

func TestRunTestDeviceNotFound(t *testing.T) {
	_, socketPath := startTestControlServerForTest(t)

	var buf strings.Builder
	err := runTestDevice(socketPath, "nonexistent-device", "", 0, &buf)
	if err == nil {
		t.Fatal("expected error for nonexistent device")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention device not found, got: %v", err)
	}
}

func TestRunTestDeviceMissingIP(t *testing.T) {
	reg, socketPath := startTestControlServerForTest(t)

	reg.Add(daemon.Device{
		ID:              "dev-no-ip",
		Name:            "No IP Phone",
		CertFingerprint: "fp-no-ip",
		Source:          daemon.SourceRuntimePaired,
	})

	var buf strings.Builder
	err := runTestDevice(socketPath, "dev-no-ip", "", 0, &buf)
	if err == nil {
		t.Fatal("expected error for device without IP")
	}

	if !strings.Contains(err.Error(), "unreachable") {
		t.Errorf("error should mention unreachable, got: %v", err)
	}
}

func TestRunTestDeviceMissingCerts(t *testing.T) {
	reg, socketPath := startTestControlServerForTest(t)

	reg.Add(daemon.Device{
		ID:              "dev-no-certs",
		Name:            "No Certs Phone",
		TailscaleIP:     "100.64.0.5",
		ListenPort:      29418,
		CertFingerprint: "fp-no-certs",
		Source:          daemon.SourceRuntimePaired,
	})

	var buf strings.Builder
	err := runTestDevice(socketPath, "dev-no-certs", "", 0, &buf)
	if err == nil {
		t.Fatal("expected error for device without cert paths")
	}

	if !strings.Contains(err.Error(), "cert") {
		t.Errorf("error should mention cert issue, got: %v", err)
	}
}

func TestRunTestDeviceDaemonUnreachable(t *testing.T) {
	var buf strings.Builder
	err := runTestDevice("/tmp/nonexistent-socket.sock", "some-device", "", 0, &buf)
	if err == nil {
		t.Fatal("expected error when daemon is unreachable")
	}

	if !strings.Contains(err.Error(), "failed to query daemon") {
		t.Errorf("error should mention daemon connection failure, got: %v", err)
	}
}

func TestRunTestDeviceConnectionRefused(t *testing.T) {
	reg, socketPath := startTestControlServerForTest(t)

	// Device with non-routable IP — connection will fail
	reg.Add(daemon.Device{
		ID:              "dev-unreachable",
		Name:            "Unreachable Phone",
		TailscaleIP:     "192.0.2.1", // TEST-NET, not routable
		ListenPort:      29418,
		CertFingerprint: "fp-unreachable",
		ClientCertPath:  "/nonexistent/client.crt",
		ClientKeyPath:   "/nonexistent/client.key",
		Source:          daemon.SourceRuntimePaired,
	})

	var buf strings.Builder
	err := runTestDevice(socketPath, "dev-unreachable", "", 0, &buf)
	if err == nil {
		t.Fatal("expected error for unreachable device")
	}

	// Should get a cert loading error since the cert files don't exist
	errStr := err.Error()
	if !strings.Contains(errStr, "unreachable") && !strings.Contains(errStr, "cert") && !strings.Contains(errStr, "no such file") {
		t.Errorf("error should indicate connectivity or cert issue, got: %v", err)
	}
}
