package mtls_test

import (
	"crypto/tls"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/phaedrus-raznikov/nix-key/internal/mtls"
)

// BenchmarkMTLSHandshake measures the time for a full mTLS handshake
// (cert generation not included — only the TLS handshake over loopback).
func BenchmarkMTLSHandshake(b *testing.B) {
	// Pre-generate server and client certs.
	serverCertPEM, serverKeyPEM, err := mtls.GenerateCert(mtls.CertOptions{
		KeyType:    mtls.KeyTypeEd25519,
		CommonName: "bench-server",
	})
	if err != nil {
		b.Fatalf("generating server cert: %v", err)
	}
	clientCertPEM, clientKeyPEM, err := mtls.GenerateCert(mtls.CertOptions{
		KeyType:    mtls.KeyTypeEd25519,
		CommonName: "bench-client",
	})
	if err != nil {
		b.Fatalf("generating client cert: %v", err)
	}

	serverFP, err := mtls.CertFingerprint(serverCertPEM)
	if err != nil {
		b.Fatalf("server fingerprint: %v", err)
	}
	clientFP, err := mtls.CertFingerprint(clientCertPEM)
	if err != nil {
		b.Fatalf("client fingerprint: %v", err)
	}

	serverTLS, err := mtls.PinnedTLSConfig(mtls.PinnedTLSOptions{
		CertPEM:         serverCertPEM,
		KeyPEM:          serverKeyPEM,
		PeerFingerprint: clientFP,
		IsServer:        true,
	})
	if err != nil {
		b.Fatalf("server TLS config: %v", err)
	}

	clientTLS, err := mtls.PinnedTLSConfig(mtls.PinnedTLSOptions{
		CertPEM:         clientCertPEM,
		KeyPEM:          clientKeyPEM,
		PeerFingerprint: serverFP,
		IsServer:        false,
	})
	if err != nil {
		b.Fatalf("client TLS config: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchHandshake(b, serverTLS, clientTLS)
	}
}

func benchHandshake(b *testing.B, serverTLS, clientTLS *tls.Config) {
	b.Helper()

	// Create a TCP connection pair via loopback.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("listen: %v", err)
	}
	defer lis.Close()

	errCh := make(chan error, 1)
	go func() {
		conn, err := lis.Accept()
		if err != nil {
			errCh <- err
			return
		}
		tlsConn := tls.Server(conn, serverTLS)
		err = tlsConn.Handshake()
		_ = tlsConn.Close()
		errCh <- err
	}()

	clientConn, err := net.Dial("tcp", lis.Addr().String())
	if err != nil {
		b.Fatalf("dial: %v", err)
	}
	tlsClient := tls.Client(clientConn, clientTLS)
	if err := tlsClient.Handshake(); err != nil {
		b.Fatalf("client handshake: %v", err)
	}
	_ = tlsClient.Close()

	if err := <-errCh; err != nil {
		b.Fatalf("server handshake: %v", err)
	}
}

// BenchmarkAgeDecrypt measures the time to decrypt an age-encrypted file.
func BenchmarkAgeDecrypt(b *testing.B) {
	dir := b.TempDir()
	identityPath := filepath.Join(dir, "identity.txt")
	if err := mtls.GenerateIdentity(identityPath); err != nil {
		b.Fatalf("generating identity: %v", err)
	}

	// Create and encrypt a test file (simulating a PEM private key).
	plainPath := filepath.Join(dir, "secret.pem")
	plaintext := make([]byte, 256) // typical PEM key size
	for i := range plaintext {
		plaintext[i] = byte(i % 256)
	}
	if err := os.WriteFile(plainPath, plaintext, 0600); err != nil {
		b.Fatalf("writing plaintext: %v", err)
	}
	if err := mtls.EncryptFile(plainPath, identityPath); err != nil {
		b.Fatalf("encrypting: %v", err)
	}
	encPath := plainPath + ".age"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := mtls.DecryptToMemory(encPath, identityPath)
		if err != nil {
			b.Fatalf("decrypt: %v", err)
		}
	}
}
