package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/phaedrus-raznikov/nix-key/internal/daemon"
	"github.com/phaedrus-raznikov/nix-key/internal/mtls"
)

func startTestControlServerForStatus(t *testing.T, keyLister func() []daemon.KeyInfo) (*daemon.Registry, string) {
	t.Helper()

	dir := t.TempDir()
	socketPath := filepath.Join(dir, "control.sock")
	devicesPath := filepath.Join(dir, "devices.json")

	reg := daemon.NewRegistry()

	if keyLister == nil {
		keyLister = func() []daemon.KeyInfo { return nil }
	}

	srv := daemon.NewControlServer(daemon.ControlServerConfig{
		SocketPath:  socketPath,
		Registry:    reg,
		DevicesPath: devicesPath,
		KeyLister:   keyLister,
	})

	if err := srv.Start(); err != nil {
		t.Fatalf("start control server: %v", err)
	}
	t.Cleanup(func() { srv.Stop() })

	return reg, socketPath
}

func TestRunStatusBasic(t *testing.T) {
	keys := []daemon.KeyInfo{
		{Fingerprint: "SHA256:abc", KeyType: "ssh-ed25519", DisplayName: "key-1", DeviceID: "dev-1"},
		{Fingerprint: "SHA256:def", KeyType: "ecdsa-sha2-nistp256", DisplayName: "key-2", DeviceID: "dev-1"},
	}

	reg, socketPath := startTestControlServerForStatus(t, func() []daemon.KeyInfo { return keys })

	reg.Add(daemon.Device{
		ID:              "dev-1",
		Name:            "Test Phone",
		TailscaleIP:     "100.64.0.2",
		ListenPort:      29418,
		CertFingerprint: "fp-1",
		Source:          daemon.SourceRuntimePaired,
	})

	var buf strings.Builder
	err := runStatus(socketPath, &buf)
	if err != nil {
		t.Fatalf("runStatus: %v", err)
	}

	output := buf.String()

	expectations := []string{
		"running",
		socketPath,
		"1",  // device count
		"2",  // key count
	}
	for _, want := range expectations {
		if !strings.Contains(output, want) {
			t.Errorf("output should contain %q, got:\n%s", want, output)
		}
	}
}

func TestRunStatusNoDevices(t *testing.T) {
	_, socketPath := startTestControlServerForStatus(t, nil)

	var buf strings.Builder
	err := runStatus(socketPath, &buf)
	if err != nil {
		t.Fatalf("runStatus: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "running") {
		t.Errorf("output should contain 'running', got:\n%s", output)
	}
	if !strings.Contains(output, "0") {
		t.Errorf("output should contain '0' for device/key count, got:\n%s", output)
	}
	// No cert warnings section.
	if strings.Contains(output, "Certificate warnings") {
		t.Errorf("output should not have cert warnings, got:\n%s", output)
	}
}

func TestRunStatusDaemonUnreachable(t *testing.T) {
	var buf strings.Builder
	err := runStatus("/tmp/nonexistent-status-socket.sock", &buf)
	if err == nil {
		t.Fatal("expected error when daemon is unreachable")
	}
	if !strings.Contains(err.Error(), "not running") {
		t.Errorf("error should mention daemon not running, got: %v", err)
	}
}

func TestRunStatusCertExpiryWarning(t *testing.T) {
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

	// Generate a cert that expires in 10 days.
	certPEM, _, err := mtls.GenerateCert(mtls.CertOptions{
		KeyType:    mtls.KeyTypeEd25519,
		CommonName: "test-phone",
		Expiry:     10 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("generate cert: %v", err)
	}

	certPath := filepath.Join(dir, "phone-cert.pem")
	if err := os.WriteFile(certPath, certPEM, 0600); err != nil {
		t.Fatalf("write cert: %v", err)
	}

	reg.Add(daemon.Device{
		ID:              "dev-expiring",
		Name:            "Expiring Phone",
		TailscaleIP:     "100.64.0.5",
		ListenPort:      29418,
		CertFingerprint: "fp-expiring",
		CertPath:        certPath,
		Source:          daemon.SourceRuntimePaired,
	})

	var buf strings.Builder
	err = runStatus(socketPath, &buf)
	if err != nil {
		t.Fatalf("runStatus: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Certificate warnings") {
		t.Errorf("output should contain cert warnings section, got:\n%s", output)
	}
	if !strings.Contains(output, "Expiring Phone") {
		t.Errorf("output should contain device name, got:\n%s", output)
	}
	if !strings.Contains(output, "server cert") {
		t.Errorf("output should mention server cert, got:\n%s", output)
	}
}

func TestRunStatusNoCertWarningWhenFarExpiry(t *testing.T) {
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

	// Generate a cert that expires in 365 days — no warning expected.
	certPEM, _, err := mtls.GenerateCert(mtls.CertOptions{
		KeyType:    mtls.KeyTypeEd25519,
		CommonName: "test-phone",
		Expiry:     365 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("generate cert: %v", err)
	}

	certPath := filepath.Join(dir, "phone-cert.pem")
	if err := os.WriteFile(certPath, certPEM, 0600); err != nil {
		t.Fatalf("write cert: %v", err)
	}

	reg.Add(daemon.Device{
		ID:              "dev-ok",
		Name:            "OK Phone",
		TailscaleIP:     "100.64.0.6",
		ListenPort:      29418,
		CertFingerprint: "fp-ok",
		CertPath:        certPath,
		Source:          daemon.SourceRuntimePaired,
	})

	var buf strings.Builder
	err = runStatus(socketPath, &buf)
	if err != nil {
		t.Fatalf("runStatus: %v", err)
	}

	output := buf.String()
	if strings.Contains(output, "Certificate warnings") {
		t.Errorf("output should NOT contain cert warnings for far-expiry cert, got:\n%s", output)
	}
}

func TestFormatStatusOutput(t *testing.T) {
	status := &daemon.StatusInfo{
		Running:      true,
		DeviceCount:  3,
		KeyCount:     5,
		SocketPath:   "/run/user/1000/nix-key/agent.sock",
		CertWarnings: []daemon.CertWarning{},
	}

	var buf strings.Builder
	formatStatus(&buf, status)
	output := buf.String()

	if !strings.Contains(output, "running") {
		t.Errorf("should contain 'running', got:\n%s", output)
	}
	if !strings.Contains(output, "3") {
		t.Errorf("should contain device count '3', got:\n%s", output)
	}
	if !strings.Contains(output, "5") {
		t.Errorf("should contain key count '5', got:\n%s", output)
	}
	if !strings.Contains(output, "/run/user/1000/nix-key/agent.sock") {
		t.Errorf("should contain socket path, got:\n%s", output)
	}
	// No warnings section for empty warnings.
	if strings.Contains(output, "Certificate warnings") {
		t.Errorf("should NOT contain cert warnings when list is empty, got:\n%s", output)
	}
}

func TestFormatStatusWithWarnings(t *testing.T) {
	expiry := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	status := &daemon.StatusInfo{
		Running:     true,
		DeviceCount: 1,
		KeyCount:    2,
		SocketPath:  "/run/user/1000/nix-key/agent.sock",
		CertWarnings: []daemon.CertWarning{
			{DeviceID: "dev-1", DeviceName: "My Phone", CertType: "server", ExpiresAt: expiry, DaysLeft: 17},
			{DeviceID: "dev-1", DeviceName: "My Phone", CertType: "client", ExpiresAt: expiry, DaysLeft: 0},
		},
	}

	var buf strings.Builder
	formatStatus(&buf, status)
	output := buf.String()

	if !strings.Contains(output, "Certificate warnings") {
		t.Errorf("should contain cert warnings section, got:\n%s", output)
	}
	if !strings.Contains(output, "server cert") {
		t.Errorf("should mention server cert, got:\n%s", output)
	}
	if !strings.Contains(output, "expires in 17 days") {
		t.Errorf("should mention days left, got:\n%s", output)
	}
	if !strings.Contains(output, "has expired") {
		t.Errorf("should mention expired cert, got:\n%s", output)
	}
}

func TestParseStatusInfo(t *testing.T) {
	info := daemon.StatusInfo{
		Running:      true,
		DeviceCount:  2,
		KeyCount:     4,
		SocketPath:   "/tmp/test.sock",
		CertWarnings: []daemon.CertWarning{},
	}

	// Simulate JSON round-trip like ControlClient does.
	dataBytes, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	var rawData interface{}
	if err := json.Unmarshal(dataBytes, &rawData); err != nil {
		t.Fatal(err)
	}

	resp := &daemon.Response{Status: "ok", Data: rawData}
	got, err := parseStatusInfo(resp)
	if err != nil {
		t.Fatalf("parseStatusInfo: %v", err)
	}

	if !got.Running {
		t.Error("expected running=true")
	}
	if got.DeviceCount != 2 {
		t.Errorf("expected 2 devices, got %d", got.DeviceCount)
	}
	if got.KeyCount != 4 {
		t.Errorf("expected 4 keys, got %d", got.KeyCount)
	}
}

func TestParseStatusInfoErrorResponse(t *testing.T) {
	resp := &daemon.Response{Status: "error", Error: "something went wrong"}
	_, err := parseStatusInfo(resp)
	if err == nil {
		t.Fatal("expected error for error response")
	}
	if !strings.Contains(err.Error(), "something went wrong") {
		t.Errorf("error should contain daemon message, got: %v", err)
	}
}
