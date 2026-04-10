// Package mtls provides mTLS certificate generation, pinning, and age encryption.
package mtls

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"
)

// KeyType identifies the asymmetric key algorithm for cert generation.
type KeyType string

const (
	KeyTypeEd25519  KeyType = "ed25519"
	KeyTypeECDSAP256 KeyType = "ecdsa-p256"
)

// DefaultCertExpiry is the default certificate validity period (1 year).
const DefaultCertExpiry = 365 * 24 * time.Hour

// CertOptions configures certificate generation.
type CertOptions struct {
	// KeyType selects the key algorithm (ed25519 or ecdsa-p256).
	KeyType KeyType

	// Expiry is the certificate validity duration from now. Defaults to DefaultCertExpiry.
	Expiry time.Duration

	// CommonName sets the certificate subject CN. Defaults to "nix-key".
	CommonName string
}

// GenerateCert creates a self-signed X.509 certificate and private key.
// Returns PEM-encoded certificate and PKCS8-encoded private key.
func GenerateCert(opts CertOptions) (certPEM, keyPEM []byte, err error) {
	if opts.Expiry == 0 {
		opts.Expiry = DefaultCertExpiry
	}
	if opts.CommonName == "" {
		opts.CommonName = "nix-key"
	}

	// Generate key pair
	var privKey interface{}
	var pubKey interface{}

	switch opts.KeyType {
	case KeyTypeEd25519:
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, nil, fmt.Errorf("generating ed25519 key: %w", err)
		}
		privKey = priv
		pubKey = pub
	case KeyTypeECDSAP256:
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, nil, fmt.Errorf("generating ecdsa-p256 key: %w", err)
		}
		privKey = key
		pubKey = &key.PublicKey
	default:
		return nil, nil, fmt.Errorf("unsupported key type: %q", opts.KeyType)
	}

	// Generate random serial number
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, nil, fmt.Errorf("generating serial number: %w", err)
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: opts.CommonName,
		},
		NotBefore:             now,
		NotAfter:              now.Add(opts.Expiry),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Self-sign the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, pubKey, privKey)
	if err != nil {
		return nil, nil, fmt.Errorf("creating certificate: %w", err)
	}

	// Encode cert as PEM
	certPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	// Encode private key as PKCS8 PEM
	keyDER, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return nil, nil, fmt.Errorf("marshaling private key: %w", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: keyDER,
	})

	return certPEM, keyPEM, nil
}
