package pairing

import (
	"bytes"
	"encoding/pem"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/phaedrus-raznikov/nix-key/internal/daemon"
	"github.com/phaedrus-raznikov/nix-key/internal/mtls"
)

// TestIntegrationPairingIdempotency verifies that pairing the same device twice
// does not corrupt state: the second pairing overwrites the existing device entry
// without duplicating it.
func TestIntegrationPairingIdempotency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	devicesPath := filepath.Join(dir, "devices.json")
	certsDir := filepath.Join(dir, "certs")
	ageIDPath := filepath.Join(dir, "age-identity.txt")

	// Pre-create age identity via mtls.GenerateIdentity.
	if err := mtls.GenerateIdentity(ageIDPath); err != nil {
		t.Fatalf("generate age identity: %v", err)
	}

	identityBefore, err := os.ReadFile(ageIDPath)
	if err != nil {
		t.Fatal(err)
	}

	// Generate phone cert (same phone for both pairings).
	phoneCertPEM, _, err := generateClientCertPair(time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	mockEncryptor := func(plaintext []byte, identityPath string) ([]byte, error) {
		return append([]byte("ENC:"), plaintext...), nil
	}

	cfg := PairConfig{
		DevicesPath:     devicesPath,
		CertsDir:        certsDir,
		AgeIdentityPath: ageIDPath,
		Stdout:          &bytes.Buffer{},
		Encryptor:       mockEncryptor,
	}
	cfg.setDefaults()

	req := &PairingRequest{
		PhoneName:   "Idempotent Phone",
		TailscaleIP: "100.64.0.50",
		ListenPort:  29418,
		ServerCert:  string(phoneCertPEM),
		Token:       "token-1",
	}

	// First pairing.
	cert1, key1, err := generateClientCertPair(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := processPairingResult(cfg, req, cert1, key1); err != nil {
		t.Fatalf("first pairing: %v", err)
	}

	devices1, err := daemon.LoadDevicesFromJSON(devicesPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices1) != 1 {
		t.Fatalf("expected 1 device after first pair, got %d", len(devices1))
	}
	firstDeviceID := devices1[0].ID

	// Second pairing with same phone cert.
	cert2, key2, err := generateClientCertPair(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := processPairingResult(cfg, req, cert2, key2); err != nil {
		t.Fatalf("second pairing: %v", err)
	}

	devices2, err := daemon.LoadDevicesFromJSON(devicesPath)
	if err != nil {
		t.Fatal(err)
	}

	// Same phone cert → same device ID. Should be exactly 1 with that ID.
	var matchCount int
	for _, d := range devices2 {
		if d.ID == firstDeviceID {
			matchCount++
		}
	}
	if matchCount != 1 {
		t.Errorf("expected exactly 1 device with ID %q, got %d (total: %d)",
			firstDeviceID, matchCount, len(devices2))
	}

	// Age identity should NOT be regenerated.
	identityAfter, err := os.ReadFile(ageIDPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(identityBefore, identityAfter) {
		t.Error("age identity should not be regenerated on second pairing")
	}
}

// TestIntegrationEnsureAgeIdentityIdempotent verifies that ensureAgeIdentity
// creates the identity on first call and skips on subsequent calls.
func TestIntegrationEnsureAgeIdentityIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if _, err := osexec.LookPath("age-keygen"); err != nil {
		t.Skip("age-keygen not available")
	}

	dir := t.TempDir()
	idPath := filepath.Join(dir, "new-subdir", "identity.txt")

	// First call — creates identity.
	if err := ensureAgeIdentity(idPath); err != nil {
		t.Fatalf("first ensureAgeIdentity: %v", err)
	}

	content1, err := os.ReadFile(idPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(content1) == 0 {
		t.Fatal("identity file should not be empty")
	}

	// Second call — should be idempotent (file unchanged).
	if err := ensureAgeIdentity(idPath); err != nil {
		t.Fatalf("second ensureAgeIdentity: %v", err)
	}

	content2, err := os.ReadFile(idPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(content1, content2) {
		t.Error("ensureAgeIdentity should not regenerate existing identity")
	}
}

// TestIntegrationSecretsAtRest verifies that private keys stored on disk after
// pairing are age-encrypted and can be decrypted back to memory.
func TestIntegrationSecretsAtRest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	certsDir := filepath.Join(dir, "certs")
	devicesPath := filepath.Join(dir, "devices.json")
	ageIDPath := filepath.Join(dir, "age-identity.txt")

	if err := mtls.GenerateIdentity(ageIDPath); err != nil {
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

	// Use real age encryption via mtls package.
	realEncryptor := func(plaintext []byte, identityPath string) ([]byte, error) {
		tmpFile := filepath.Join(dir, "tmp-encrypt.pem")
		if err := os.WriteFile(tmpFile, plaintext, 0600); err != nil {
			return nil, err
		}
		if err := mtls.EncryptFile(tmpFile, identityPath); err != nil {
			return nil, err
		}
		return os.ReadFile(tmpFile + ".age")
	}

	cfg := PairConfig{
		DevicesPath:     devicesPath,
		CertsDir:        certsDir,
		AgeIdentityPath: ageIDPath,
		Stdout:          &bytes.Buffer{},
		Encryptor:       realEncryptor,
	}
	cfg.setDefaults()

	req := &PairingRequest{
		PhoneName:   "Secrets Phone",
		TailscaleIP: "100.64.0.60",
		ListenPort:  29418,
		ServerCert:  string(phoneCertPEM),
		Token:       "secret-token",
	}

	if err := processPairingResult(cfg, req, clientCertPEM, clientKeyPEM); err != nil {
		t.Fatalf("processPairingResult: %v", err)
	}

	devices, err := daemon.LoadDevicesFromJSON(devicesPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	dev := devices[0]

	// Encrypted key path should end in .age.
	if !strings.HasSuffix(dev.ClientKeyPath, ".age") {
		t.Errorf("client key path should end in .age, got %q", dev.ClientKeyPath)
	}

	// Encrypted data should NOT be decodable as PEM (it's age-encrypted binary).
	encryptedData, err := os.ReadFile(dev.ClientKeyPath)
	if err != nil {
		t.Fatal(err)
	}
	if block, _ := pem.Decode(encryptedData); block != nil {
		t.Error("encrypted key file is plaintext PEM — should be age-encrypted")
	}

	// Encrypted file should have 0600 permissions.
	keyInfo, err := os.Stat(dev.ClientKeyPath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := keyInfo.Mode().Perm(); perm != 0600 {
		t.Errorf("encrypted key permissions: got %o, want 0600", perm)
	}

	// Decrypt to memory should recover original key.
	decrypted, err := mtls.DecryptToMemory(dev.ClientKeyPath, ageIDPath)
	if err != nil {
		t.Fatalf("DecryptToMemory: %v", err)
	}
	if !bytes.Equal(decrypted, clientKeyPEM) {
		t.Error("decrypted key does not match original")
	}

	// Public certs should be plaintext.
	serverCert, err := os.ReadFile(dev.CertPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(serverCert, []byte("-----BEGIN CERTIFICATE-----")) {
		t.Error("server cert should be plaintext PEM")
	}

	hostCert, err := os.ReadFile(dev.ClientCertPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(hostCert, []byte("-----BEGIN CERTIFICATE-----")) {
		t.Error("host client cert should be plaintext PEM")
	}
}

// TestIntegrationPairingIdempotencyWithRealAge tests the full pairing
// idempotency flow with real age CLI tools.
func TestIntegrationPairingIdempotencyWithRealAge(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
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

	cfg := PairConfig{
		DevicesPath:     devicesPath,
		CertsDir:        certsDir,
		AgeIdentityPath: ageIDPath,
		Stdout:          &bytes.Buffer{},
	}
	cfg.setDefaults()

	phoneCertPEM, _, err := generateClientCertPair(time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	req := &PairingRequest{
		PhoneName:   "Real Age Phone",
		TailscaleIP: "100.64.0.70",
		ListenPort:  29418,
		ServerCert:  string(phoneCertPEM),
		Token:       "real-token-1",
	}

	// First pair — creates age identity + encrypted key.
	cert1, key1, err := generateClientCertPair(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := processPairingResult(cfg, req, cert1, key1); err != nil {
		t.Fatalf("first pair: %v", err)
	}

	identityAfterFirst, err := os.ReadFile(ageIDPath)
	if err != nil {
		t.Fatal(err)
	}

	// Second pair — re-pair same phone.
	cert2, key2, err := generateClientCertPair(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := processPairingResult(cfg, req, cert2, key2); err != nil {
		t.Fatalf("second pair: %v", err)
	}

	// Identity should be unchanged.
	identityAfterSecond, err := os.ReadFile(ageIDPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(identityAfterFirst, identityAfterSecond) {
		t.Error("age identity should not change between pairings")
	}

	// The encrypted key from second pairing should be decryptable.
	devices, err := daemon.LoadDevicesFromJSON(devicesPath)
	if err != nil {
		t.Fatal(err)
	}

	var found bool
	for _, d := range devices {
		if d.ClientKeyPath != "" {
			encData, err := os.ReadFile(d.ClientKeyPath)
			if err != nil {
				t.Fatalf("read encrypted key: %v", err)
			}
			if block, _ := pem.Decode(encData); block != nil {
				t.Error("key on disk should be encrypted, not plaintext PEM")
			}

			cmd := osexec.Command("age", "-d", "-i", ageIDPath)
			cmd.Stdin = bytes.NewReader(encData)
			decrypted, err := cmd.Output()
			if err != nil {
				t.Fatalf("age decrypt: %v", err)
			}
			if !bytes.Equal(decrypted, key2) {
				t.Error("decrypted key should match second pairing's key")
			}
			found = true
		}
	}
	if !found {
		t.Error("no device with encrypted key found")
	}
}
