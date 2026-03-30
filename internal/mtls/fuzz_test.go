package mtls

import (
	"crypto/x509"
	"encoding/pem"
	"testing"
)

// FuzzCertPEMParse tests that arbitrary bytes don't panic when parsed as
// a PEM-encoded X.509 certificate.
func FuzzCertPEMParse(f *testing.F) {
	// Generate a valid cert for the seed corpus
	certPEM, _, err := GenerateCert(CertOptions{KeyType: KeyTypeEd25519})
	if err == nil {
		f.Add(certPEM)
	}
	f.Add([]byte("this is not a PEM encoded certificate"))
	f.Add([]byte{})
	f.Add([]byte("-----BEGIN CERTIFICATE-----\n-----END CERTIFICATE-----"))
	f.Add([]byte("-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----"))

	f.Fuzz(func(t *testing.T, data []byte) {
		block, _ := pem.Decode(data)
		if block == nil {
			return
		}
		_, _ = x509.ParseCertificate(block.Bytes)
	})
}

// FuzzCertFingerprint tests that CertFingerprint handles arbitrary bytes
// without panicking.
func FuzzCertFingerprint(f *testing.F) {
	certPEM, _, err := GenerateCert(CertOptions{KeyType: KeyTypeEd25519})
	if err == nil {
		f.Add(certPEM)
	}
	f.Add([]byte("not pem"))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = CertFingerprint(data)
	})
}
