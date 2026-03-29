package pairing

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/phaedrus-raznikov/nix-key/internal/daemon"
)

// safeBuffer is a goroutine-safe wrapper around bytes.Buffer.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (sb *safeBuffer) Write(p []byte) (int, error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

func (sb *safeBuffer) String() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.String()
}

// TestPairTailscaleUnavailable verifies FR-E11: nix-key pair fails cleanly
// when the Tailscale interface is not available.
func TestPairTailscaleUnavailable(t *testing.T) {
	cfg := PairConfig{
		TailscaleInterface: "nonexistent_iface_12345",
	}

	err := RunPair(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error when Tailscale interface is unavailable")
	}
	if !strings.Contains(err.Error(), "tailscale interface") {
		t.Errorf("error should mention tailscale interface, got: %v", err)
	}
	if !strings.Contains(err.Error(), "nonexistent_iface_12345") {
		t.Errorf("error should mention interface name, got: %v", err)
	}
	if !strings.Contains(err.Error(), "unavailable") {
		t.Errorf("error should mention 'unavailable', got: %v", err)
	}
}

// TestPairTailscaleUnavailableDefaultResolver verifies FR-E11 with the real
// interface resolver (not mocked).
func TestPairTailscaleUnavailableDefaultResolver(t *testing.T) {
	_, err := getTailscaleIP("nonexistent_iface_99999")
	if err == nil {
		t.Fatal("expected error for nonexistent interface")
	}
	if !strings.Contains(err.Error(), "unavailable") {
		t.Errorf("expected 'unavailable' in error, got: %v", err)
	}
}

// TestGenerateClientCertPair verifies host client cert generation.
func TestGenerateClientCertPair(t *testing.T) {
	certPEM, keyPEM, err := generateClientCertPair(24 * time.Hour)
	if err != nil {
		t.Fatalf("generateClientCertPair: %v", err)
	}

	// Verify cert is valid PEM
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		t.Fatal("cert PEM decode failed")
	}
	if certBlock.Type != "CERTIFICATE" {
		t.Errorf("cert PEM type = %q, want CERTIFICATE", certBlock.Type)
	}

	// Verify key is valid PEM
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		t.Fatal("key PEM decode failed")
	}
	if keyBlock.Type != "EC PRIVATE KEY" {
		t.Errorf("key PEM type = %q, want EC PRIVATE KEY", keyBlock.Type)
	}

	// Verify TLS keypair loads
	_, err = tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("X509KeyPair: %v", err)
	}
}

// TestGenerateToken verifies token generation produces unique values.
func TestGenerateToken(t *testing.T) {
	t1, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken: %v", err)
	}
	t2, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken: %v", err)
	}

	if len(t1) != 64 { // 32 bytes hex-encoded
		t.Errorf("token length = %d, want 64", len(t1))
	}
	if t1 == t2 {
		t.Error("two tokens should be different")
	}
}

// TestCertFingerprint verifies SHA256 fingerprint computation.
func TestCertFingerprint(t *testing.T) {
	certPEM, _, err := generateClientCertPair(time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	fp, err := certFingerprint(certPEM)
	if err != nil {
		t.Fatalf("certFingerprint: %v", err)
	}

	if len(fp) != 64 { // SHA256 hex
		t.Errorf("fingerprint length = %d, want 64", len(fp))
	}

	// Verify it matches manual computation
	block, _ := pem.Decode(certPEM)
	hash := sha256.Sum256(block.Bytes)
	expected := hex.EncodeToString(hash[:])
	if fp != expected {
		t.Errorf("fingerprint = %q, want %q", fp, expected)
	}
}

// TestCertFingerprintInvalidPEM verifies error on invalid PEM.
func TestCertFingerprintInvalidPEM(t *testing.T) {
	_, err := certFingerprint([]byte("not a PEM"))
	if err == nil {
		t.Error("expected error for invalid PEM")
	}
}

// TestPromptConfirmYes verifies the confirmation prompt accepts "y".
func TestPromptConfirmYes(t *testing.T) {
	var out bytes.Buffer
	in := strings.NewReader("y\n")

	req := PairingRequest{
		PhoneName:   "Pixel 8",
		TailscaleIP: "100.64.0.2",
		ListenPort:  29418,
	}

	result := promptConfirm(&out, in, req)
	if !result {
		t.Error("expected approval for 'y' input")
	}
	if !strings.Contains(out.String(), "Pixel 8") {
		t.Error("output should contain phone name")
	}
	if !strings.Contains(out.String(), "100.64.0.2") {
		t.Error("output should contain Tailscale IP")
	}
}

// TestPromptConfirmNo verifies the confirmation prompt rejects "n".
func TestPromptConfirmNo(t *testing.T) {
	var out bytes.Buffer
	in := strings.NewReader("n\n")

	req := PairingRequest{PhoneName: "Test", TailscaleIP: "1.2.3.4", ListenPort: 1234}
	result := promptConfirm(&out, in, req)
	if result {
		t.Error("expected denial for 'n' input")
	}
}

// TestPromptConfirmDefault verifies empty input defaults to deny.
func TestPromptConfirmDefault(t *testing.T) {
	var out bytes.Buffer
	in := strings.NewReader("\n")

	req := PairingRequest{PhoneName: "Test", TailscaleIP: "1.2.3.4", ListenPort: 1234}
	result := promptConfirm(&out, in, req)
	if result {
		t.Error("expected denial for empty input (default N)")
	}
}

// TestExtractAgeRecipient verifies public key extraction from age identity.
func TestExtractAgeRecipient(t *testing.T) {
	dir := t.TempDir()
	idPath := filepath.Join(dir, "identity.txt")

	content := `# created: 2026-01-01T00:00:00Z
# public key: age1testpublickey12345
AGE-SECRET-KEY-1TESTSECRETKEY12345
`
	if err := os.WriteFile(idPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	recipient, err := extractAgeRecipient(idPath)
	if err != nil {
		t.Fatalf("extractAgeRecipient: %v", err)
	}
	if recipient != "age1testpublickey12345" {
		t.Errorf("recipient = %q, want %q", recipient, "age1testpublickey12345")
	}
}

// TestExtractAgeRecipientMissing verifies error when identity file has no public key.
func TestExtractAgeRecipientMissing(t *testing.T) {
	dir := t.TempDir()
	idPath := filepath.Join(dir, "identity.txt")
	if err := os.WriteFile(idPath, []byte("AGE-SECRET-KEY-1FOO\n"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := extractAgeRecipient(idPath)
	if err == nil {
		t.Error("expected error when public key is missing")
	}
}

// TestEnsureAgeIdentity verifies age identity file generation.
func TestEnsureAgeIdentity(t *testing.T) {
	if _, err := osexec.LookPath("age-keygen"); err != nil {
		t.Skip("age-keygen not available")
	}

	dir := t.TempDir()
	idPath := filepath.Join(dir, "subdir", "identity.txt")

	if err := ensureAgeIdentity(idPath); err != nil {
		t.Fatalf("ensureAgeIdentity: %v", err)
	}

	// Verify file exists and contains a public key
	_, err := extractAgeRecipient(idPath)
	if err != nil {
		t.Errorf("generated identity should have public key: %v", err)
	}

	// Verify idempotent: calling again should not error
	if err := ensureAgeIdentity(idPath); err != nil {
		t.Errorf("second call should succeed: %v", err)
	}
}

// TestAgeEncryptDecrypt verifies age encryption round-trip.
func TestAgeEncryptDecrypt(t *testing.T) {
	if _, err := osexec.LookPath("age"); err != nil {
		t.Skip("age not available")
	}
	if _, err := osexec.LookPath("age-keygen"); err != nil {
		t.Skip("age-keygen not available")
	}

	dir := t.TempDir()
	idPath := filepath.Join(dir, "identity.txt")
	if err := ensureAgeIdentity(idPath); err != nil {
		t.Fatal(err)
	}

	plaintext := []byte("test secret key data for encryption")
	ciphertext, err := ageEncrypt(plaintext, idPath)
	if err != nil {
		t.Fatalf("ageEncrypt: %v", err)
	}

	if len(ciphertext) == 0 {
		t.Fatal("ciphertext should not be empty")
	}
	if bytes.Equal(ciphertext, plaintext) {
		t.Error("ciphertext should differ from plaintext")
	}

	// Decrypt and verify round-trip
	cmd := osexec.Command("age", "-d", "-i", idPath)
	cmd.Stdin = bytes.NewReader(ciphertext)
	decrypted, err := cmd.Output()
	if err != nil {
		t.Fatalf("age decrypt: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("decrypted = %q, want %q", decrypted, plaintext)
	}
}

// TestProcessPairingResult tests the post-approval processing.
func TestProcessPairingResult(t *testing.T) {
	dir := t.TempDir()
	devicesPath := filepath.Join(dir, "devices.json")
	certsDir := filepath.Join(dir, "certs")
	ageIdPath := filepath.Join(dir, "age-identity.txt")

	os.MkdirAll(filepath.Dir(ageIdPath), 0700)
	os.WriteFile(ageIdPath, []byte("# public key: age1fake\nAGE-SECRET-KEY-1FAKE\n"), 0600)

	// Generate certs
	phoneCertPEM, _, err := generateClientCertPair(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	clientCertPEM, clientKeyPEM, err := generateClientCertPair(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	cfg := PairConfig{
		DevicesPath:     devicesPath,
		CertsDir:        certsDir,
		AgeIdentityPath: ageIdPath,
		Stdout:          &output,
		Encryptor: func(plaintext []byte, identityPath string) ([]byte, error) {
			return append([]byte("ENCRYPTED:"), plaintext...), nil
		},
	}
	cfg.setDefaults()

	req := &PairingRequest{
		PhoneName:   "Pixel 8 Pro",
		TailscaleIP: "100.64.0.42",
		ListenPort:  29418,
		ServerCert:  string(phoneCertPEM),
		Token:       "used-token",
	}

	if err := processPairingResult(cfg, req, clientCertPEM, clientKeyPEM); err != nil {
		t.Fatalf("processPairingResult: %v", err)
	}

	// Verify devices.json was created with the device
	devices, err := daemon.LoadDevicesFromJSON(devicesPath)
	if err != nil {
		t.Fatalf("load devices: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}

	dev := devices[0]
	if dev.Name != "Pixel 8 Pro" {
		t.Errorf("device name = %q, want %q", dev.Name, "Pixel 8 Pro")
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

	// Verify cert files exist
	fp, _ := certFingerprint(phoneCertPEM)
	deviceDir := filepath.Join(certsDir, fp[:16])

	if _, err := os.Stat(filepath.Join(deviceDir, "phone-server-cert.pem")); err != nil {
		t.Errorf("phone cert file should exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(deviceDir, "host-client-cert.pem")); err != nil {
		t.Errorf("host cert file should exist: %v", err)
	}

	// Verify encrypted key has mock prefix
	keyData, err := os.ReadFile(filepath.Join(deviceDir, "host-client-key.pem.age"))
	if err != nil {
		t.Fatalf("read encrypted key: %v", err)
	}
	if !bytes.HasPrefix(keyData, []byte("ENCRYPTED:")) {
		t.Error("key should be encrypted (mock prefix)")
	}
}

// TestPairIntegrationWithRealAge tests the full post-pairing processing with
// real age encryption.
func TestPairIntegrationWithRealAge(t *testing.T) {
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
	ageIdPath := filepath.Join(dir, "age-identity.txt")

	if err := ensureAgeIdentity(ageIdPath); err != nil {
		t.Fatal(err)
	}

	phoneCertPEM, _, err := generateClientCertPair(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	clientCertPEM, clientKeyPEM, err := generateClientCertPair(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	cfg := PairConfig{
		DevicesPath:     devicesPath,
		CertsDir:        certsDir,
		AgeIdentityPath: ageIdPath,
		Stdout:          &output,
	}
	cfg.setDefaults()

	req := &PairingRequest{
		PhoneName:   "Integration Phone",
		TailscaleIP: "100.64.1.1",
		ListenPort:  29418,
		ServerCert:  string(phoneCertPEM),
		Token:       "test-token",
	}

	if err := processPairingResult(cfg, req, clientCertPEM, clientKeyPEM); err != nil {
		t.Fatalf("processPairingResult: %v", err)
	}

	// Verify age-encrypted key can be decrypted
	fp, _ := certFingerprint(phoneCertPEM)
	hostKeyPath := filepath.Join(certsDir, fp[:16], "host-client-key.pem.age")
	encryptedKey, err := os.ReadFile(hostKeyPath)
	if err != nil {
		t.Fatal(err)
	}

	cmd := osexec.Command("age", "-d", "-i", ageIdPath)
	cmd.Stdin = bytes.NewReader(encryptedKey)
	decrypted, err := cmd.Output()
	if err != nil {
		t.Fatalf("age decrypt: %v", err)
	}
	if !bytes.Equal(decrypted, clientKeyPEM) {
		t.Error("decrypted key should match original")
	}
}

// TestPairServerStartsAndOutputs tests that RunPair starts a server on the
// loopback and displays expected output.
func TestPairServerStartsAndOutputs(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	var output safeBuffer

	cfg := PairConfig{
		TailscaleInterface: "loopback",
		CertExpiry:         24 * time.Hour,
		HostName:           "test-host",
		OTELEndpoint:       "100.64.0.1:4317",
		Stdout:             &output,
		Stdin:              strings.NewReader(""),
		InterfaceResolver: func(name string) (string, error) {
			return "127.0.0.1", nil
		},
		ConfirmFunc: func(req PairingRequest) bool {
			return false
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go RunPair(ctx, cfg)

	// Wait for output
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		if strings.Contains(output.String(), "Waiting for phone") {
			break
		}
	}

	out := output.String()
	if !strings.Contains(out, "test-host") {
		t.Error("output should contain hostname")
	}
	if !strings.Contains(out, "nix-key pairing") {
		t.Error("output should contain pairing header")
	}
	if !strings.Contains(out, "100.64.0.1:4317") {
		t.Error("output should contain OTEL endpoint")
	}
	if !strings.Contains(out, "Listening on port") {
		t.Error("output should show listening port")
	}
	if !strings.Contains(out, "Scan this QR code") {
		t.Error("output should prompt to scan QR code")
	}

	cancel()
}

// TestServerClosedErrorDetection verifies the isServerClosed helper.
func TestServerClosedErrorDetection(t *testing.T) {
	if !isServerClosed(http.ErrServerClosed) {
		t.Error("should detect http.ErrServerClosed")
	}
	if isServerClosed(nil) {
		t.Error("should not flag nil as server closed")
	}
	if isServerClosed(fmt.Errorf("some other error")) {
		t.Error("should not flag unrelated error as server closed")
	}
}

// TestGetTailscaleIPLoopback verifies IP extraction from a real interface.
func TestGetTailscaleIPLoopback(t *testing.T) {
	ip, err := getTailscaleIP("lo")
	if err != nil {
		t.Skipf("loopback interface test: %v", err)
	}
	if ip != "127.0.0.1" {
		t.Errorf("loopback IP = %q, want 127.0.0.1", ip)
	}
}
