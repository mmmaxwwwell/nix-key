package mtls

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"
)

func TestGenerateCert_Ed25519_Defaults(t *testing.T) {
	certPEM, keyPEM, err := GenerateCert(CertOptions{
		KeyType: KeyTypeEd25519,
	})
	if err != nil {
		t.Fatalf("GenerateCert failed: %v", err)
	}

	cert, key := decodeCertAndKey(t, certPEM, keyPEM)

	// Verify key type
	if _, ok := key.(ed25519.PrivateKey); !ok {
		t.Fatalf("expected ed25519.PrivateKey, got %T", key)
	}

	// Verify self-signed
	if cert.Issuer.CommonName != cert.Subject.CommonName {
		t.Errorf("expected self-signed cert, issuer=%q subject=%q", cert.Issuer.CommonName, cert.Subject.CommonName)
	}

	// Verify default expiry (~1 year)
	expectedExpiry := time.Now().Add(DefaultCertExpiry)
	if cert.NotAfter.Before(expectedExpiry.Add(-time.Minute)) || cert.NotAfter.After(expectedExpiry.Add(time.Minute)) {
		t.Errorf("expected expiry ~%v, got %v", expectedExpiry, cert.NotAfter)
	}

	// Verify cert is valid now
	if time.Now().Before(cert.NotBefore) || time.Now().After(cert.NotAfter) {
		t.Error("cert is not currently valid")
	}

	// Verify cert can be verified against itself (self-signed)
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	if _, err := cert.Verify(x509.VerifyOptions{Roots: pool}); err != nil {
		t.Errorf("self-signed cert verification failed: %v", err)
	}
}

func TestGenerateCert_ECDSAP256(t *testing.T) {
	certPEM, keyPEM, err := GenerateCert(CertOptions{
		KeyType: KeyTypeECDSAP256,
	})
	if err != nil {
		t.Fatalf("GenerateCert failed: %v", err)
	}

	cert, key := decodeCertAndKey(t, certPEM, keyPEM)

	ecKey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		t.Fatalf("expected *ecdsa.PrivateKey, got %T", key)
	}
	if ecKey.Curve != elliptic.P256() {
		t.Errorf("expected P-256 curve, got %v", ecKey.Curve.Params().Name)
	}

	// Verify cert public key matches
	certECKey, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		t.Fatalf("cert public key is not ECDSA: %T", cert.PublicKey)
	}
	if certECKey.Curve != elliptic.P256() {
		t.Errorf("cert public key curve is not P-256")
	}
}

func TestGenerateCert_CustomExpiry(t *testing.T) {
	expiry := 30 * 24 * time.Hour // 30 days
	certPEM, _, err := GenerateCert(CertOptions{
		KeyType: KeyTypeEd25519,
		Expiry:  expiry,
	})
	if err != nil {
		t.Fatalf("GenerateCert failed: %v", err)
	}

	cert := decodeCert(t, certPEM)

	expectedExpiry := time.Now().Add(expiry)
	if cert.NotAfter.Before(expectedExpiry.Add(-time.Minute)) || cert.NotAfter.After(expectedExpiry.Add(time.Minute)) {
		t.Errorf("expected expiry ~%v, got %v", expectedExpiry, cert.NotAfter)
	}
}

func TestGenerateCert_CustomCommonName(t *testing.T) {
	certPEM, _, err := GenerateCert(CertOptions{
		KeyType:    KeyTypeEd25519,
		CommonName: "test-device.nix-key",
	})
	if err != nil {
		t.Fatalf("GenerateCert failed: %v", err)
	}

	cert := decodeCert(t, certPEM)

	if cert.Subject.CommonName != "test-device.nix-key" {
		t.Errorf("expected CN=%q, got %q", "test-device.nix-key", cert.Subject.CommonName)
	}
}

func TestGenerateCert_PEMFormat(t *testing.T) {
	certPEM, keyPEM, err := GenerateCert(CertOptions{
		KeyType: KeyTypeEd25519,
	})
	if err != nil {
		t.Fatalf("GenerateCert failed: %v", err)
	}

	// Verify cert PEM block type
	certBlock, rest := pem.Decode(certPEM)
	if certBlock == nil {
		t.Fatal("failed to decode cert PEM")
	}
	if certBlock.Type != "CERTIFICATE" {
		t.Errorf("expected PEM type CERTIFICATE, got %q", certBlock.Type)
	}
	if len(rest) != 0 {
		t.Error("unexpected trailing data after cert PEM")
	}

	// Verify key PEM block type
	keyBlock, rest := pem.Decode(keyPEM)
	if keyBlock == nil {
		t.Fatal("failed to decode key PEM")
	}
	if keyBlock.Type != "PRIVATE KEY" {
		t.Errorf("expected PEM type PRIVATE KEY, got %q", keyBlock.Type)
	}
	if len(rest) != 0 {
		t.Error("unexpected trailing data after key PEM")
	}
}

func TestGenerateCert_CertUsageFlags(t *testing.T) {
	certPEM, _, err := GenerateCert(CertOptions{
		KeyType: KeyTypeEd25519,
	})
	if err != nil {
		t.Fatalf("GenerateCert failed: %v", err)
	}

	cert := decodeCert(t, certPEM)

	// Should be a CA cert (self-signed)
	if !cert.IsCA {
		t.Error("expected self-signed cert to be CA")
	}

	// Should have key usage for digital signature and cert signing
	if cert.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		t.Error("expected KeyUsageDigitalSignature")
	}

	// Should support both client and server auth
	hasClientAuth := false
	hasServerAuth := false
	for _, usage := range cert.ExtKeyUsage {
		if usage == x509.ExtKeyUsageClientAuth {
			hasClientAuth = true
		}
		if usage == x509.ExtKeyUsageServerAuth {
			hasServerAuth = true
		}
	}
	if !hasClientAuth {
		t.Error("expected ExtKeyUsageClientAuth")
	}
	if !hasServerAuth {
		t.Error("expected ExtKeyUsageServerAuth")
	}
}

func TestGenerateCert_InvalidKeyType(t *testing.T) {
	_, _, err := GenerateCert(CertOptions{
		KeyType: "rsa-4096",
	})
	if err == nil {
		t.Fatal("expected error for invalid key type")
	}
}

func TestGenerateCert_UniqueSerialNumbers(t *testing.T) {
	certPEM1, _, err := GenerateCert(CertOptions{KeyType: KeyTypeEd25519})
	if err != nil {
		t.Fatalf("first GenerateCert failed: %v", err)
	}
	certPEM2, _, err := GenerateCert(CertOptions{KeyType: KeyTypeEd25519})
	if err != nil {
		t.Fatalf("second GenerateCert failed: %v", err)
	}

	cert1 := decodeCert(t, certPEM1)
	cert2 := decodeCert(t, certPEM2)

	if cert1.SerialNumber.Cmp(cert2.SerialNumber) == 0 {
		t.Error("expected unique serial numbers for different certs")
	}
}

// decodeCertAndKey parses PEM-encoded cert and key for test assertions.
func decodeCertAndKey(t *testing.T, certPEM, keyPEM []byte) (*x509.Certificate, interface{}) {
	t.Helper()

	cert := decodeCert(t, certPEM)

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		t.Fatal("failed to decode key PEM")
	}
	key, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		t.Fatalf("failed to parse private key: %v", err)
	}

	return cert, key
}

// decodeCert parses a PEM-encoded certificate for test assertions.
func decodeCert(t *testing.T, certPEM []byte) *x509.Certificate {
	t.Helper()

	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("failed to decode cert PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("failed to parse certificate: %v", err)
	}
	return cert
}
