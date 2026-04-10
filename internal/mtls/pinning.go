package mtls

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"time"
)

// CertFingerprint computes the SHA256 fingerprint of a PEM-encoded certificate.
// Returns the fingerprint as a lowercase hex string.
func CertFingerprint(certPEM []byte) (string, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return "", fmt.Errorf("failed to decode PEM certificate")
	}
	hash := sha256.Sum256(block.Bytes)
	return hex.EncodeToString(hash[:]), nil
}

// PinnedTLSOptions configures a pinned TLS connection.
type PinnedTLSOptions struct {
	// CertPEM is the local certificate in PEM format.
	CertPEM []byte

	// KeyPEM is the local private key in PEM format.
	KeyPEM []byte

	// PeerFingerprint is the expected SHA256 fingerprint of the peer's certificate.
	PeerFingerprint string

	// IsServer indicates whether this config is for a TLS server (true) or client (false).
	IsServer bool
}

// PinnedTLSConfig creates a tls.Config that verifies the peer certificate's
// SHA256 fingerprint matches the expected value. It also rejects expired certificates.
// Works for both client and server modes based on IsServer.
func PinnedTLSConfig(opts PinnedTLSOptions) (*tls.Config, error) {
	cert, err := tls.X509KeyPair(opts.CertPEM, opts.KeyPEM)
	if err != nil {
		return nil, fmt.Errorf("loading key pair: %w", err)
	}

	verifyPeer := func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			return fmt.Errorf("peer provided no certificate")
		}

		peerCert, err := x509.ParseCertificate(rawCerts[0])
		if err != nil {
			return fmt.Errorf("parsing peer certificate: %w", err)
		}

		// Check expiry
		now := time.Now()
		if now.Before(peerCert.NotBefore) || now.After(peerCert.NotAfter) {
			return fmt.Errorf("peer certificate expired or not yet valid (notBefore=%s, notAfter=%s)",
				peerCert.NotBefore.Format(time.RFC3339), peerCert.NotAfter.Format(time.RFC3339))
		}

		// Check fingerprint
		hash := sha256.Sum256(rawCerts[0])
		gotFP := hex.EncodeToString(hash[:])
		if gotFP != opts.PeerFingerprint {
			return fmt.Errorf("peer certificate fingerprint mismatch: got %s, want %s", gotFP, opts.PeerFingerprint)
		}

		return nil
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
		NextProtos:   []string{"h2"}, // Required for gRPC ALPN negotiation
	}

	if opts.IsServer {
		tlsConfig.ClientAuth = tls.RequireAnyClientCert
		tlsConfig.VerifyPeerCertificate = verifyPeer
	} else {
		// Skip standard verification (self-signed certs); we use fingerprint pinning instead
		tlsConfig.InsecureSkipVerify = true
		tlsConfig.VerifyPeerCertificate = verifyPeer
	}

	return tlsConfig, nil
}
