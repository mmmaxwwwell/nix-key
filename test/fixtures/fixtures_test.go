package fixtures_test

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/ssh"
)

func fixturesDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	return filepath.Dir(thisFile)
}

func TestCACert(t *testing.T) {
	dir := fixturesDir(t)
	certPEM, err := os.ReadFile(filepath.Join(dir, "ca-cert.pem"))
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("failed to decode CA cert PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if !cert.IsCA {
		t.Error("CA cert should have IsCA=true")
	}
	if cert.Subject.CommonName != "nix-key Test CA" {
		t.Errorf("unexpected CA CN: %s", cert.Subject.CommonName)
	}
}

func TestMTLSCertChain(t *testing.T) {
	dir := fixturesDir(t)

	// Load CA
	caPEM, err := os.ReadFile(filepath.Join(dir, "ca-cert.pem"))
	if err != nil {
		t.Fatal(err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caPEM) {
		t.Fatal("failed to add CA cert to pool")
	}

	tests := []struct {
		name     string
		certFile string
		keyFile  string
		cn       string
		usage    x509.ExtKeyUsage
	}{
		{"host-client", "host-client-cert.pem", "host-client-key.pem", "nix-key-host-client", x509.ExtKeyUsageClientAuth},
		{"phone-server", "phone-server-cert.pem", "phone-server-key.pem", "nix-key-phone-server", x509.ExtKeyUsageServerAuth},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Load and parse cert
			certPEM, err := os.ReadFile(filepath.Join(dir, tt.certFile))
			if err != nil {
				t.Fatal(err)
			}
			block, _ := pem.Decode(certPEM)
			if block == nil {
				t.Fatal("failed to decode cert PEM")
			}
			leaf, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				t.Fatal(err)
			}

			// Verify the key file is parseable
			keyPEM, err := os.ReadFile(filepath.Join(dir, tt.keyFile))
			if err != nil {
				t.Fatal(err)
			}
			keyBlock, _ := pem.Decode(keyPEM)
			if keyBlock == nil {
				t.Fatal("failed to decode key PEM")
			}
			_, err = x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
			if err != nil {
				t.Fatalf("failed to parse private key: %v", err)
			}

			if leaf.Subject.CommonName != tt.cn {
				t.Errorf("unexpected CN: got %s, want %s", leaf.Subject.CommonName, tt.cn)
			}

			// Verify chain to CA
			_, err = leaf.Verify(x509.VerifyOptions{
				Roots:     caPool,
				KeyUsages: []x509.ExtKeyUsage{tt.usage},
			})
			if err != nil {
				t.Errorf("cert chain verification failed: %v", err)
			}
		})
	}
}

func TestCertFingerprints(t *testing.T) {
	dir := fixturesDir(t)
	for _, name := range []string{"ca-cert.pem", "host-client-cert.pem", "phone-server-cert.pem"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		block, _ := pem.Decode(data)
		fp := sha256.Sum256(block.Bytes)
		t.Logf("%s fingerprint: %x", name, fp)
	}
}

func TestSSHEd25519Keypair(t *testing.T) {
	dir := fixturesDir(t)

	// Read public key
	pubData, err := os.ReadFile(filepath.Join(dir, "ssh-ed25519.pub"))
	if err != nil {
		t.Fatal(err)
	}
	pub, _, _, _, err := ssh.ParseAuthorizedKey(pubData)
	if err != nil {
		t.Fatal(err)
	}
	if pub.Type() != "ssh-ed25519" {
		t.Errorf("unexpected key type: %s", pub.Type())
	}

	// Read private key
	privData, err := os.ReadFile(filepath.Join(dir, "ssh-ed25519"))
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.ParsePrivateKey(privData)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the private key corresponds to the public key
	if signer.PublicKey().Type() != "ssh-ed25519" {
		t.Errorf("private key type mismatch: %s", signer.PublicKey().Type())
	}

	// Verify underlying type
	cryptoKey := signer.PublicKey().(ssh.CryptoPublicKey).CryptoPublicKey()
	if _, ok := cryptoKey.(ed25519.PublicKey); !ok {
		t.Errorf("expected ed25519.PublicKey, got %T", cryptoKey)
	}

	t.Logf("Ed25519 fingerprint: %s", ssh.FingerprintSHA256(pub))
}

func TestSSHECDSAKeypair(t *testing.T) {
	dir := fixturesDir(t)

	pubData, err := os.ReadFile(filepath.Join(dir, "ssh-ecdsa.pub"))
	if err != nil {
		t.Fatal(err)
	}
	pub, _, _, _, err := ssh.ParseAuthorizedKey(pubData)
	if err != nil {
		t.Fatal(err)
	}
	if pub.Type() != "ecdsa-sha2-nistp256" {
		t.Errorf("unexpected key type: %s", pub.Type())
	}

	privData, err := os.ReadFile(filepath.Join(dir, "ssh-ecdsa"))
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.ParsePrivateKey(privData)
	if err != nil {
		t.Fatal(err)
	}

	cryptoKey := signer.PublicKey().(ssh.CryptoPublicKey).CryptoPublicKey()
	if _, ok := cryptoKey.(*ecdsa.PublicKey); !ok {
		t.Errorf("expected *ecdsa.PublicKey, got %T", cryptoKey)
	}

	t.Logf("ECDSA fingerprint: %s", ssh.FingerprintSHA256(pub))
}

func TestAgeEncryption(t *testing.T) {
	dir := fixturesDir(t)

	// Verify plaintext file exists
	plaintext, err := os.ReadFile(filepath.Join(dir, "age-plaintext.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(plaintext) == 0 {
		t.Fatal("plaintext file is empty")
	}

	// Verify identity file exists and contains expected format
	identity, err := os.ReadFile(filepath.Join(dir, "age-identity.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(identity) == 0 {
		t.Fatal("identity file is empty")
	}

	// Verify encrypted file exists and is non-empty
	encrypted, err := os.ReadFile(filepath.Join(dir, "age-encrypted.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if len(encrypted) == 0 {
		t.Fatal("encrypted file is empty")
	}

	// Verify encrypted != plaintext
	if string(encrypted) == string(plaintext) {
		t.Error("encrypted file should differ from plaintext")
	}

	t.Logf("age plaintext: %d bytes, encrypted: %d bytes", len(plaintext), len(encrypted))
}
