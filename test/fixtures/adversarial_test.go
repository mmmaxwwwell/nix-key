package fixtures_test

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func adversarialDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(fixturesDir(t), "adversarial")
}

func loadCert(t *testing.T, path string) *x509.Certificate {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cert %s: %v", path, err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		t.Fatalf("decode PEM %s: no block found", path)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse cert %s: %v", path, err)
	}
	return cert
}

func loadKey(t *testing.T, path string) interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read key %s: %v", path, err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		t.Fatalf("decode PEM %s: no block found", path)
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Fatalf("parse key %s: %v", path, err)
	}
	return key
}

func TestAdversarialExpiredCert(t *testing.T) {
	dir := adversarialDir(t)
	cert := loadCert(t, filepath.Join(dir, "expired-client-cert.pem"))
	_ = loadKey(t, filepath.Join(dir, "expired-client-key.pem"))

	if cert.Subject.CommonName != "adv-expired-client" {
		t.Errorf("unexpected CN: %s", cert.Subject.CommonName)
	}
	if cert.NotAfter.After(time.Now()) {
		t.Error("expired cert should have NotAfter in the past")
	}

	// Verify it's signed by the legitimate CA but fails time validation
	caPool := loadCAPool(t)
	_, err := cert.Verify(x509.VerifyOptions{
		Roots:       caPool,
		KeyUsages:   []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		CurrentTime: time.Now(),
	})
	if err == nil {
		t.Error("expired cert should fail verification")
	}
}

func TestAdversarialFutureCert(t *testing.T) {
	dir := adversarialDir(t)
	cert := loadCert(t, filepath.Join(dir, "future-client-cert.pem"))
	_ = loadKey(t, filepath.Join(dir, "future-client-key.pem"))

	if cert.Subject.CommonName != "adv-future-client" {
		t.Errorf("unexpected CN: %s", cert.Subject.CommonName)
	}
	if cert.NotBefore.Before(time.Now()) {
		t.Error("future cert should have NotBefore in the future")
	}

	caPool := loadCAPool(t)
	_, err := cert.Verify(x509.VerifyOptions{
		Roots:       caPool,
		KeyUsages:   []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		CurrentTime: time.Now(),
	})
	if err == nil {
		t.Error("future cert should fail verification")
	}
}

func TestAdversarialWrongCACert(t *testing.T) {
	dir := adversarialDir(t)
	cert := loadCert(t, filepath.Join(dir, "wrong-ca-client-cert.pem"))
	_ = loadKey(t, filepath.Join(dir, "wrong-ca-client-key.pem"))
	rogueCA := loadCert(t, filepath.Join(dir, "rogue-ca-cert.pem"))
	_ = loadKey(t, filepath.Join(dir, "rogue-ca-key.pem"))

	if cert.Subject.CommonName != "adv-wrong-ca-client" {
		t.Errorf("unexpected CN: %s", cert.Subject.CommonName)
	}
	if rogueCA.Subject.CommonName == "nix-key Test CA" {
		t.Error("rogue CA should have a different CN than the legitimate CA")
	}

	// Should fail against the legitimate CA pool
	caPool := loadCAPool(t)
	_, err := cert.Verify(x509.VerifyOptions{
		Roots:     caPool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	if err == nil {
		t.Error("wrong-CA cert should fail verification against legitimate CA")
	}

	// Should succeed against a pool containing the rogue CA
	roguePool := x509.NewCertPool()
	roguePool.AddCert(rogueCA)
	_, err = cert.Verify(x509.VerifyOptions{
		Roots:     roguePool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	if err != nil {
		t.Errorf("wrong-CA cert should verify against rogue CA: %v", err)
	}
}

func TestAdversarialWrongEKUCert(t *testing.T) {
	dir := adversarialDir(t)
	cert := loadCert(t, filepath.Join(dir, "wrong-eku-client-cert.pem"))
	_ = loadKey(t, filepath.Join(dir, "wrong-eku-client-key.pem"))

	if cert.Subject.CommonName != "adv-wrong-eku-client" {
		t.Errorf("unexpected CN: %s", cert.Subject.CommonName)
	}

	// Should have ServerAuth EKU, not ClientAuth
	hasServerAuth := false
	for _, eku := range cert.ExtKeyUsage {
		if eku == x509.ExtKeyUsageServerAuth {
			hasServerAuth = true
		}
		if eku == x509.ExtKeyUsageClientAuth {
			t.Error("wrong-EKU cert should NOT have ClientAuth")
		}
	}
	if !hasServerAuth {
		t.Error("wrong-EKU cert should have ServerAuth")
	}

	// Should fail verification when ClientAuth is required
	caPool := loadCAPool(t)
	_, err := cert.Verify(x509.VerifyOptions{
		Roots:     caPool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	if err == nil {
		t.Error("wrong-EKU cert should fail verification with ClientAuth requirement")
	}
}

func TestAdversarialUnpairedCert(t *testing.T) {
	dir := adversarialDir(t)
	cert := loadCert(t, filepath.Join(dir, "unpaired-client-cert.pem"))
	_ = loadKey(t, filepath.Join(dir, "unpaired-client-key.pem"))

	if cert.Subject.CommonName != "adv-unpaired-device" {
		t.Errorf("unexpected CN: %s", cert.Subject.CommonName)
	}

	// Should be a valid cert signed by the legitimate CA with correct EKU
	caPool := loadCAPool(t)
	_, err := cert.Verify(x509.VerifyOptions{
		Roots:     caPool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	if err != nil {
		t.Errorf("unpaired cert should be cryptographically valid: %v", err)
	}
}

func TestAdversarialDeterministic(t *testing.T) {
	dir := adversarialDir(t)
	files := []string{
		"expired-client-cert.pem",
		"future-client-cert.pem",
		"wrong-ca-client-cert.pem",
		"wrong-eku-client-cert.pem",
		"unpaired-client-cert.pem",
		"rogue-ca-cert.pem",
	}
	for _, f := range files {
		path := filepath.Join(dir, f)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("missing adversarial fixture %s: %v", f, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("adversarial fixture %s is empty", f)
		}
	}
}

// loadCAPool loads the legitimate CA cert pool from test/fixtures.
func loadCAPool(t *testing.T) *x509.CertPool {
	t.Helper()
	caPEM, err := os.ReadFile(filepath.Join(fixturesDir(t), "ca-cert.pem"))
	if err != nil {
		t.Fatalf("load CA cert: %v", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		t.Fatal("failed to add CA cert to pool")
	}
	return pool
}
