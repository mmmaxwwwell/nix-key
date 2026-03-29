package mtls

import (
	"crypto/sha256"
	"crypto/tls"
"encoding/hex"
	"encoding/pem"
	"fmt"
	"net"
	"testing"
	"time"
)

func TestCertFingerprint(t *testing.T) {
	certPEM, _, err := GenerateCert(CertOptions{KeyType: KeyTypeEd25519})
	if err != nil {
		t.Fatalf("GenerateCert failed: %v", err)
	}

	fp, err := CertFingerprint(certPEM)
	if err != nil {
		t.Fatalf("CertFingerprint failed: %v", err)
	}

	// Verify it's a valid hex-encoded SHA256 hash (64 hex chars)
	if len(fp) != 64 {
		t.Errorf("expected 64 hex chars, got %d: %q", len(fp), fp)
	}
	if _, err := hex.DecodeString(fp); err != nil {
		t.Errorf("fingerprint is not valid hex: %v", err)
	}

	// Verify it matches manual computation
	block, _ := pem.Decode(certPEM)
	hash := sha256.Sum256(block.Bytes)
	expected := hex.EncodeToString(hash[:])
	if fp != expected {
		t.Errorf("fingerprint mismatch: got %q, want %q", fp, expected)
	}
}

func TestCertFingerprint_InvalidPEM(t *testing.T) {
	_, err := CertFingerprint([]byte("not a pem"))
	if err == nil {
		t.Fatal("expected error for invalid PEM")
	}
}

func TestPinnedTLSConfig_MatchingFingerprint(t *testing.T) {
	// Generate server cert
	serverCertPEM, serverKeyPEM, err := GenerateCert(CertOptions{
		KeyType:    KeyTypeEd25519,
		CommonName: "server",
	})
	if err != nil {
		t.Fatalf("generating server cert: %v", err)
	}
	serverFP, err := CertFingerprint(serverCertPEM)
	if err != nil {
		t.Fatalf("server fingerprint: %v", err)
	}

	// Generate client cert
	clientCertPEM, clientKeyPEM, err := GenerateCert(CertOptions{
		KeyType:    KeyTypeEd25519,
		CommonName: "client",
	})
	if err != nil {
		t.Fatalf("generating client cert: %v", err)
	}
	clientFP, err := CertFingerprint(clientCertPEM)
	if err != nil {
		t.Fatalf("client fingerprint: %v", err)
	}

	// Set up server TLS config pinning the client cert
	serverTLS, err := PinnedTLSConfig(PinnedTLSOptions{
		CertPEM:         serverCertPEM,
		KeyPEM:          serverKeyPEM,
		PeerFingerprint: clientFP,
		IsServer:        true,
	})
	if err != nil {
		t.Fatalf("server PinnedTLSConfig: %v", err)
	}

	// Set up client TLS config pinning the server cert
	clientTLS, err := PinnedTLSConfig(PinnedTLSOptions{
		CertPEM:         clientCertPEM,
		KeyPEM:          clientKeyPEM,
		PeerFingerprint: serverFP,
		IsServer:        false,
	})
	if err != nil {
		t.Fatalf("client PinnedTLSConfig: %v", err)
	}

	// Perform mTLS handshake — should succeed
	if err := doTLSHandshake(t, serverTLS, clientTLS); err != nil {
		t.Fatalf("handshake with matching fingerprints should succeed: %v", err)
	}
}

func TestPinnedTLSConfig_WrongFingerprint(t *testing.T) {
	// Generate server cert
	serverCertPEM, serverKeyPEM, err := GenerateCert(CertOptions{
		KeyType:    KeyTypeEd25519,
		CommonName: "server",
	})
	if err != nil {
		t.Fatalf("generating server cert: %v", err)
	}

	// Generate client cert
	clientCertPEM, clientKeyPEM, err := GenerateCert(CertOptions{
		KeyType:    KeyTypeEd25519,
		CommonName: "client",
	})
	if err != nil {
		t.Fatalf("generating client cert: %v", err)
	}

	// Use a wrong fingerprint for the server's view of client
	wrongFP := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	serverTLS, err := PinnedTLSConfig(PinnedTLSOptions{
		CertPEM:         serverCertPEM,
		KeyPEM:          serverKeyPEM,
		PeerFingerprint: wrongFP,
		IsServer:        true,
	})
	if err != nil {
		t.Fatalf("server PinnedTLSConfig: %v", err)
	}

	// Client pins the correct server fingerprint
	serverFP, _ := CertFingerprint(serverCertPEM)
	clientTLS, err := PinnedTLSConfig(PinnedTLSOptions{
		CertPEM:         clientCertPEM,
		KeyPEM:          clientKeyPEM,
		PeerFingerprint: serverFP,
		IsServer:        false,
	})
	if err != nil {
		t.Fatalf("client PinnedTLSConfig: %v", err)
	}

	// Handshake should fail because server rejects client's fingerprint
	if err := doTLSHandshake(t, serverTLS, clientTLS); err == nil {
		t.Fatal("handshake with wrong fingerprint should fail")
	}
}

func TestPinnedTLSConfig_ExpiredCert(t *testing.T) {
	// Generate an already-expired server cert (expired 1 hour ago)
	serverCertPEM, serverKeyPEM, err := GenerateCert(CertOptions{
		KeyType:    KeyTypeEd25519,
		CommonName: "expired-server",
		Expiry:     -1 * time.Hour, // negative expiry = already expired
	})
	if err != nil {
		t.Fatalf("generating expired server cert: %v", err)
	}
	serverFP, err := CertFingerprint(serverCertPEM)
	if err != nil {
		t.Fatalf("server fingerprint: %v", err)
	}

	// Generate valid client cert
	clientCertPEM, clientKeyPEM, err := GenerateCert(CertOptions{
		KeyType:    KeyTypeEd25519,
		CommonName: "client",
	})
	if err != nil {
		t.Fatalf("generating client cert: %v", err)
	}
	clientFP, err := CertFingerprint(clientCertPEM)
	if err != nil {
		t.Fatalf("client fingerprint: %v", err)
	}

	serverTLS, err := PinnedTLSConfig(PinnedTLSOptions{
		CertPEM:         serverCertPEM,
		KeyPEM:          serverKeyPEM,
		PeerFingerprint: clientFP,
		IsServer:        true,
	})
	if err != nil {
		t.Fatalf("server PinnedTLSConfig: %v", err)
	}

	clientTLS, err := PinnedTLSConfig(PinnedTLSOptions{
		CertPEM:         clientCertPEM,
		KeyPEM:          clientKeyPEM,
		PeerFingerprint: serverFP,
		IsServer:        false,
	})
	if err != nil {
		t.Fatalf("client PinnedTLSConfig: %v", err)
	}

	// Handshake should fail because server cert is expired
	if err := doTLSHandshake(t, serverTLS, clientTLS); err == nil {
		t.Fatal("handshake with expired server cert should fail")
	}
}

func TestPinnedTLSConfig_ClientRejectsWrongServer(t *testing.T) {
	// Generate two server certs — client pins one, actual server uses the other
	serverCertPEM, serverKeyPEM, err := GenerateCert(CertOptions{
		KeyType:    KeyTypeEd25519,
		CommonName: "real-server",
	})
	if err != nil {
		t.Fatalf("generating real server cert: %v", err)
	}

	otherCertPEM, _, err := GenerateCert(CertOptions{
		KeyType:    KeyTypeEd25519,
		CommonName: "imposter-server",
	})
	if err != nil {
		t.Fatalf("generating imposter cert: %v", err)
	}
	// Client pins the imposter's fingerprint, not the real server
	otherFP, _ := CertFingerprint(otherCertPEM)

	clientCertPEM, clientKeyPEM, err := GenerateCert(CertOptions{
		KeyType:    KeyTypeEd25519,
		CommonName: "client",
	})
	if err != nil {
		t.Fatalf("generating client cert: %v", err)
	}
	clientFP, _ := CertFingerprint(clientCertPEM)

	serverTLS, err := PinnedTLSConfig(PinnedTLSOptions{
		CertPEM:         serverCertPEM,
		KeyPEM:          serverKeyPEM,
		PeerFingerprint: clientFP,
		IsServer:        true,
	})
	if err != nil {
		t.Fatalf("server PinnedTLSConfig: %v", err)
	}

	clientTLS, err := PinnedTLSConfig(PinnedTLSOptions{
		CertPEM:         clientCertPEM,
		KeyPEM:          clientKeyPEM,
		PeerFingerprint: otherFP, // pins the wrong server
		IsServer:        false,
	})
	if err != nil {
		t.Fatalf("client PinnedTLSConfig: %v", err)
	}

	// Handshake should fail because client rejects the real server's fingerprint
	if err := doTLSHandshake(t, serverTLS, clientTLS); err == nil {
		t.Fatal("handshake should fail when client pins wrong server fingerprint")
	}
}

// doTLSHandshake performs a full mTLS handshake between server and client configs.
// Returns nil on success, error on failure.
func doTLSHandshake(t *testing.T, serverConfig, clientConfig *tls.Config) error {
	t.Helper()

	ln, err := tls.Listen("tcp", "127.0.0.1:0", serverConfig)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer ln.Close()

	errCh := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			errCh <- fmt.Errorf("accept: %w", err)
			return
		}
		defer conn.Close()

		tlsConn := conn.(*tls.Conn)
		tlsConn.SetDeadline(time.Now().Add(5 * time.Second))
		if err := tlsConn.Handshake(); err != nil {
			errCh <- fmt.Errorf("server handshake: %w", err)
			return
		}
		errCh <- nil
	}()

	addr := ln.Addr().(*net.TCPAddr)
	conn, err := tls.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", addr.Port), clientConfig)
	if err != nil {
		// Wait for server error too
		<-errCh
		return fmt.Errorf("client dial: %w", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))
	if err := conn.Handshake(); err != nil {
		<-errCh
		return fmt.Errorf("client handshake: %w", err)
	}

	// Wait for server side result
	if serverErr := <-errCh; serverErr != nil {
		return serverErr
	}

	return nil
}
