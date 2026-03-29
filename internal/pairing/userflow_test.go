package pairing

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	osexec "os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/phaedrus-raznikov/nix-key/internal/daemon"
)

// TestIntegrationPairingUserFlowApproved is the happy-path user-flow test (SC-002):
// start daemon (control server), assemble pairing components, simulated phone HTTP
// client completes pairing. Verify: device in registry, certs encrypted with age,
// control socket notified daemon, list-devices shows the new device.
func TestIntegrationPairingUserFlowApproved(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	if _, err := osexec.LookPath("age"); err != nil {
		t.Skip("age not available")
	}
	if _, err := osexec.LookPath("age-keygen"); err != nil {
		t.Skip("age-keygen not available")
	}

	dir := t.TempDir()
	devicesPath := filepath.Join(dir, "devices.json")
	certsDir := filepath.Join(dir, "certs")
	ageIDPath := filepath.Join(dir, "age-identity.txt")
	socketPath := filepath.Join(dir, "control.sock")

	// Step 1: Generate age identity for encrypting keys.
	if err := ensureAgeIdentity(ageIDPath); err != nil {
		t.Fatalf("create age identity: %v", err)
	}

	// Step 2: Start daemon (control server).
	reg := daemon.NewRegistry()
	ctrlSrv := daemon.NewControlServer(daemon.ControlServerConfig{
		SocketPath:  socketPath,
		Registry:    reg,
		DevicesPath: devicesPath,
		KeyLister:   func() []daemon.KeyInfo { return nil },
	})
	if err := ctrlSrv.Start(); err != nil {
		t.Fatalf("start control server: %v", err)
	}
	defer ctrlSrv.Stop()

	// Step 3: Generate host client cert pair (same as RunPair step 2).
	clientCertPEM, clientKeyPEM, err := generateClientCertPair(24 * time.Hour)
	if err != nil {
		t.Fatalf("generate host client cert: %v", err)
	}

	// Step 4: Generate one-time token (same as RunPair step 3).
	token, err := generateToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	// Step 5: Create and start pairing server (same as RunPair step 4).
	pairSrv, err := NewPairingServer(PairingServerConfig{
		Token:          token,
		HostName:       "test-host",
		HostClientCert: string(clientCertPEM),
		PairingTimeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("create pairing server: %v", err)
	}
	pairSrv.SetConfirmCallback(func(req PairingRequest) bool {
		return true // Auto-approve
	})

	ln, err := tls.Listen("tcp", "127.0.0.1:0", pairSrv.TLSConfig())
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = pairSrv.Serve(ln) }()
	defer func() { _ = pairSrv.Shutdown(context.Background()) }()

	port := ln.Addr().(*net.TCPAddr).Port

	// Step 6: Generate simulated phone's server cert.
	phoneCertPEM, _, err := generateClientCertPair(time.Hour)
	if err != nil {
		t.Fatalf("generate phone cert: %v", err)
	}

	// Step 7: Simulated phone POSTs to pairing endpoint.
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 10 * time.Second,
	}

	phoneReq := PairingRequest{
		PhoneName:   "Test Pixel",
		TailscaleIP: "100.64.0.42",
		ListenPort:  29418,
		ServerCert:  string(phoneCertPEM),
		Token:       token,
	}
	body, _ := json.Marshal(phoneReq)

	resp, err := httpClient.Post(
		fmt.Sprintf("https://127.0.0.1:%d/pair", port),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("phone POST /pair: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, respBody)
	}

	var pairResp PairingResponse
	if err := json.NewDecoder(resp.Body).Decode(&pairResp); err != nil {
		t.Fatalf("decode pairing response: %v", err)
	}
	if pairResp.Status != "approved" {
		t.Fatalf("expected status=approved, got %q", pairResp.Status)
	}
	if pairResp.HostName != "test-host" {
		t.Errorf("expected hostName=test-host, got %q", pairResp.HostName)
	}
	if pairResp.HostClientCert != string(clientCertPEM) {
		t.Error("response should contain the host client cert")
	}

	// Step 8: Process pairing result (same as RunPair step 7).
	completedReq := pairSrv.GetCompletedRequest()
	if completedReq == nil {
		t.Fatal("expected completed request from pairing server")
	}

	var pairOutput bytes.Buffer
	pairCfg := PairConfig{
		DevicesPath:       devicesPath,
		CertsDir:          certsDir,
		AgeIdentityPath:   ageIDPath,
		ControlSocketPath: socketPath,
		Stdout:            &pairOutput,
	}
	pairCfg.setDefaults()

	if err := processPairingResult(pairCfg, completedReq, clientCertPEM, clientKeyPEM); err != nil {
		t.Fatalf("process pairing result: %v", err)
	}

	// ===== VERIFICATION =====

	// Verify 1: Device is in devices.json.
	devices, err := daemon.LoadDevicesFromJSON(devicesPath)
	if err != nil {
		t.Fatalf("load devices.json: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device in devices.json, got %d", len(devices))
	}
	dev := devices[0]
	if dev.Name != "Test Pixel" {
		t.Errorf("device name = %q, want %q", dev.Name, "Test Pixel")
	}
	if dev.TailscaleIP != "100.64.0.42" {
		t.Errorf("device IP = %q, want %q", dev.TailscaleIP, "100.64.0.42")
	}
	if dev.ListenPort != 29418 {
		t.Errorf("device port = %d, want %d", dev.ListenPort, 29418)
	}
	if dev.Source != daemon.SourceRuntimePaired {
		t.Errorf("device source = %q, want %q", dev.Source, daemon.SourceRuntimePaired)
	}

	// Verify 2: Certs encrypted with age.
	fp, _ := certFingerprint(phoneCertPEM)
	deviceDir := filepath.Join(certsDir, fp[:16])

	// Phone server cert (plaintext public cert).
	phoneCertOnDisk, err := os.ReadFile(filepath.Join(deviceDir, "phone-server-cert.pem"))
	if err != nil {
		t.Fatalf("read phone cert: %v", err)
	}
	if string(phoneCertOnDisk) != string(phoneCertPEM) {
		t.Error("phone cert on disk doesn't match original")
	}

	// Host client cert (plaintext public cert).
	hostCertOnDisk, err := os.ReadFile(filepath.Join(deviceDir, "host-client-cert.pem"))
	if err != nil {
		t.Fatalf("read host cert: %v", err)
	}
	if string(hostCertOnDisk) != string(clientCertPEM) {
		t.Error("host cert on disk doesn't match original")
	}

	// Host client key (age-encrypted).
	encryptedKeyPath := filepath.Join(deviceDir, "host-client-key.pem.age")
	encryptedKey, err := os.ReadFile(encryptedKeyPath)
	if err != nil {
		t.Fatalf("read encrypted key: %v", err)
	}
	if len(encryptedKey) == 0 {
		t.Fatal("encrypted key file is empty")
	}
	if !bytes.Contains(encryptedKey, []byte("age-encryption.org")) {
		t.Error("key file should be age-encrypted")
	}

	// Decrypt and verify round-trip.
	cmd := osexec.Command("age", "-d", "-i", ageIDPath)
	cmd.Stdin = bytes.NewReader(encryptedKey)
	decryptedKey, err := cmd.Output()
	if err != nil {
		t.Fatalf("decrypt host client key: %v", err)
	}
	if !bytes.Equal(decryptedKey, clientKeyPEM) {
		t.Error("decrypted key doesn't match original client key")
	}

	// Verify 3: Control socket notified daemon (device in registry).
	time.Sleep(100 * time.Millisecond)
	allDevices := reg.ListAll()
	if len(allDevices) != 1 {
		t.Fatalf("expected 1 device in daemon registry, got %d", len(allDevices))
	}
	if allDevices[0].Name != "Test Pixel" {
		t.Errorf("daemon registry device name = %q, want %q", allDevices[0].Name, "Test Pixel")
	}

	// Verify 4: list-devices via control socket shows the new device.
	client := daemon.NewControlClient(socketPath)
	listResp, err := client.SendCommand(daemon.Request{Command: "list-devices"})
	if err != nil {
		t.Fatalf("list-devices command: %v", err)
	}
	if listResp.Status != "ok" {
		t.Fatalf("list-devices status = %q, error = %q", listResp.Status, listResp.Error)
	}

	listData, _ := json.Marshal(listResp.Data)
	var deviceInfos []daemon.DeviceInfo
	if err := json.Unmarshal(listData, &deviceInfos); err != nil {
		t.Fatalf("unmarshal device list: %v", err)
	}
	if len(deviceInfos) != 1 {
		t.Fatalf("list-devices returned %d devices, want 1", len(deviceInfos))
	}
	if deviceInfos[0].Name != "Test Pixel" {
		t.Errorf("listed device name = %q, want %q", deviceInfos[0].Name, "Test Pixel")
	}
	if deviceInfos[0].TailscaleIP != "100.64.0.42" {
		t.Errorf("listed device IP = %q, want %q", deviceInfos[0].TailscaleIP, "100.64.0.42")
	}
	if deviceInfos[0].Source != string(daemon.SourceRuntimePaired) {
		t.Errorf("listed device source = %q, want %q", deviceInfos[0].Source, daemon.SourceRuntimePaired)
	}
}

// TestIntegrationPairingDenied tests that when pairing is denied, no state is saved.
func TestIntegrationPairingDenied(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	dir := t.TempDir()
	devicesPath := filepath.Join(dir, "devices.json")
	certsDir := filepath.Join(dir, "certs")
	socketPath := filepath.Join(dir, "control.sock")

	// Start daemon control server.
	reg := daemon.NewRegistry()
	ctrlSrv := daemon.NewControlServer(daemon.ControlServerConfig{
		SocketPath:  socketPath,
		Registry:    reg,
		DevicesPath: devicesPath,
		KeyLister:   func() []daemon.KeyInfo { return nil },
	})
	if err := ctrlSrv.Start(); err != nil {
		t.Fatalf("start control server: %v", err)
	}
	defer ctrlSrv.Stop()

	// Generate certs and token.
	clientCertPEM, _, err := generateClientCertPair(24 * time.Hour)
	if err != nil {
		t.Fatalf("generate host client cert: %v", err)
	}

	token, err := generateToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	// Create pairing server with DENY callback.
	pairSrv, err := NewPairingServer(PairingServerConfig{
		Token:          token,
		HostName:       "test-host",
		HostClientCert: string(clientCertPEM),
		PairingTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("create pairing server: %v", err)
	}
	pairSrv.SetConfirmCallback(func(req PairingRequest) bool {
		return false // DENY
	})

	ln, err := tls.Listen("tcp", "127.0.0.1:0", pairSrv.TLSConfig())
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = pairSrv.Serve(ln) }()
	defer func() { _ = pairSrv.Shutdown(context.Background()) }()

	port := ln.Addr().(*net.TCPAddr).Port

	phoneCertPEM, _, err := generateClientCertPair(time.Hour)
	if err != nil {
		t.Fatalf("generate phone cert: %v", err)
	}

	// Simulated phone POSTs.
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 10 * time.Second,
	}

	phoneReq := PairingRequest{
		PhoneName:   "Denied Phone",
		TailscaleIP: "100.64.0.99",
		ListenPort:  29418,
		ServerCert:  string(phoneCertPEM),
		Token:       token,
	}
	body, _ := json.Marshal(phoneReq)

	resp, err := httpClient.Post(
		fmt.Sprintf("https://127.0.0.1:%d/pair", port),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("phone POST /pair: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for denied pairing, got %d", resp.StatusCode)
	}

	var pairResp PairingResponse
	if err := json.NewDecoder(resp.Body).Decode(&pairResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if pairResp.Status != "denied" {
		t.Errorf("expected status=denied, got %q", pairResp.Status)
	}

	// Verify: No completed request.
	if pairSrv.GetCompletedRequest() != nil {
		t.Error("expected no completed request after denial")
	}

	// Verify: No devices.json created.
	if _, err := os.Stat(devicesPath); !os.IsNotExist(err) {
		t.Error("devices.json should not exist after denied pairing")
	}

	// Verify: No certs directory created.
	if _, err := os.Stat(certsDir); !os.IsNotExist(err) {
		t.Error("certs directory should not exist after denied pairing")
	}

	// Verify: Daemon registry is empty.
	if len(reg.ListAll()) != 0 {
		t.Error("daemon registry should be empty after denied pairing")
	}

	// Verify: list-devices via control socket returns empty.
	client := daemon.NewControlClient(socketPath)
	listResp, err := client.SendCommand(daemon.Request{Command: "list-devices"})
	if err != nil {
		t.Fatalf("list-devices: %v", err)
	}
	listData, _ := json.Marshal(listResp.Data)
	var deviceInfos []daemon.DeviceInfo
	_ = json.Unmarshal(listData, &deviceInfos)
	if len(deviceInfos) != 0 {
		t.Errorf("list-devices should return 0 devices after denial, got %d", len(deviceInfos))
	}
}

// TestIntegrationPairingTokenReplay tests that replaying a used token is rejected (FR-E10).
func TestIntegrationPairingTokenReplay(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	if _, err := osexec.LookPath("age"); err != nil {
		t.Skip("age not available")
	}
	if _, err := osexec.LookPath("age-keygen"); err != nil {
		t.Skip("age-keygen not available")
	}

	dir := t.TempDir()
	devicesPath := filepath.Join(dir, "devices.json")
	certsDir := filepath.Join(dir, "certs")
	ageIDPath := filepath.Join(dir, "age-identity.txt")
	socketPath := filepath.Join(dir, "control.sock")

	if err := ensureAgeIdentity(ageIDPath); err != nil {
		t.Fatalf("create age identity: %v", err)
	}

	// Start daemon.
	reg := daemon.NewRegistry()
	ctrlSrv := daemon.NewControlServer(daemon.ControlServerConfig{
		SocketPath:  socketPath,
		Registry:    reg,
		DevicesPath: devicesPath,
		KeyLister:   func() []daemon.KeyInfo { return nil },
	})
	if err := ctrlSrv.Start(); err != nil {
		t.Fatalf("start control server: %v", err)
	}
	defer ctrlSrv.Stop()

	// Generate host certs and token.
	clientCertPEM, clientKeyPEM, err := generateClientCertPair(24 * time.Hour)
	if err != nil {
		t.Fatalf("generate host client cert: %v", err)
	}

	token, err := generateToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	// Create pairing server with auto-approve.
	pairSrv, err := NewPairingServer(PairingServerConfig{
		Token:          token,
		HostName:       "test-host",
		HostClientCert: string(clientCertPEM),
		PairingTimeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("create pairing server: %v", err)
	}
	pairSrv.SetConfirmCallback(func(req PairingRequest) bool {
		return true
	})

	ln, err := tls.Listen("tcp", "127.0.0.1:0", pairSrv.TLSConfig())
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = pairSrv.Serve(ln) }()
	defer func() { _ = pairSrv.Shutdown(context.Background()) }()

	port := ln.Addr().(*net.TCPAddr).Port

	phoneCertPEM, _, err := generateClientCertPair(time.Hour)
	if err != nil {
		t.Fatalf("generate phone cert: %v", err)
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 10 * time.Second,
	}

	// First request: should succeed.
	phoneReq := PairingRequest{
		PhoneName:   "Legit Phone",
		TailscaleIP: "100.64.0.10",
		ListenPort:  29418,
		ServerCert:  string(phoneCertPEM),
		Token:       token,
	}
	body, _ := json.Marshal(phoneReq)

	resp1, err := httpClient.Post(
		fmt.Sprintf("https://127.0.0.1:%d/pair", port),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("first POST: %v", err)
	}
	resp1.Body.Close()
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", resp1.StatusCode)
	}

	// Process the first pairing to register the device.
	completedReq := pairSrv.GetCompletedRequest()
	if completedReq == nil {
		t.Fatal("expected completed request after first pairing")
	}

	var pairOutput bytes.Buffer
	pairCfg := PairConfig{
		DevicesPath:       devicesPath,
		CertsDir:          certsDir,
		AgeIdentityPath:   ageIDPath,
		ControlSocketPath: socketPath,
		Stdout:            &pairOutput,
	}
	pairCfg.setDefaults()

	if err := processPairingResult(pairCfg, completedReq, clientCertPEM, clientKeyPEM); err != nil {
		t.Fatalf("process first pairing result: %v", err)
	}

	// Verify first device was registered.
	devices, _ := daemon.LoadDevicesFromJSON(devicesPath)
	if len(devices) != 1 {
		t.Fatalf("expected 1 device after first pairing, got %d", len(devices))
	}

	// Wait briefly for the server to auto-shutdown after consuming the token.
	time.Sleep(100 * time.Millisecond)

	// Second request with same token (replay): should be REJECTED.
	// The server shuts down after consuming the one-time token, so the replay
	// attempt either gets a 401 or a connection refused. Both prevent reuse.
	body2, _ := json.Marshal(PairingRequest{
		PhoneName:   "Attacker Phone",
		TailscaleIP: "100.64.0.99",
		ListenPort:  29418,
		ServerCert:  string(phoneCertPEM),
		Token:       token, // Replay same token
	})

	resp2, err := httpClient.Post(
		fmt.Sprintf("https://127.0.0.1:%d/pair", port),
		"application/json",
		bytes.NewReader(body2),
	)
	if err == nil {
		defer resp2.Body.Close()
		if resp2.StatusCode != http.StatusUnauthorized {
			t.Fatalf("replay request: expected 401 or connection refused, got %d", resp2.StatusCode)
		}
	}
	// If err != nil, the server is already shut down — replay rejected via connection refusal.

	// Verify: Still only 1 device (attacker not registered).
	devices2, _ := daemon.LoadDevicesFromJSON(devicesPath)
	if len(devices2) != 1 {
		t.Fatalf("expected still 1 device after replay, got %d", len(devices2))
	}
	if devices2[0].Name != "Legit Phone" {
		t.Errorf("device should be Legit Phone, got %q", devices2[0].Name)
	}

	// Verify via control socket: only 1 device.
	time.Sleep(100 * time.Millisecond)
	client := daemon.NewControlClient(socketPath)
	listResp, err := client.SendCommand(daemon.Request{Command: "list-devices"})
	if err != nil {
		t.Fatalf("list-devices: %v", err)
	}
	listData, _ := json.Marshal(listResp.Data)
	var deviceInfos []daemon.DeviceInfo
	_ = json.Unmarshal(listData, &deviceInfos)
	if len(deviceInfos) != 1 {
		t.Fatalf("daemon should have 1 device after replay, got %d", len(deviceInfos))
	}
	if deviceInfos[0].Name != "Legit Phone" {
		t.Errorf("daemon device should be Legit Phone, got %q", deviceInfos[0].Name)
	}
}
