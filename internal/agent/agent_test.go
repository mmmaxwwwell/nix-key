package agent_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	sshagent "golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh"

	"github.com/phaedrus-raznikov/nix-key/internal/agent"
)

// mockBackend implements agent.Backend for testing.
type mockBackend struct {
	keys    []*sshagent.Key
	signFn  func(key ssh.PublicKey, data []byte, flags sshagent.SignatureFlags) (*ssh.Signature, error)
	listErr error
	signErr error
}

func (m *mockBackend) List() ([]*sshagent.Key, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.keys, nil
}

func (m *mockBackend) Sign(key ssh.PublicKey, data []byte, flags sshagent.SignatureFlags) (*ssh.Signature, error) {
	if m.signFn != nil {
		return m.signFn(key, data, flags)
	}
	if m.signErr != nil {
		return nil, m.signErr
	}
	return &ssh.Signature{
		Format: key.Type(),
		Blob:   []byte("mock-signature"),
	}, nil
}

func newTestKey(t *testing.T) (*sshagent.Key, ssh.Signer) {
	t.Helper()
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(privKey)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	pub := signer.PublicKey()
	return &sshagent.Key{
		Format:  pub.Type(),
		Blob:    pub.Marshal(),
		Comment: "test-key",
	}, signer
}

func startTestAgent(t *testing.T, backend agent.Backend) (sshagent.ExtendedAgent, func()) {
	t.Helper()
	socketDir := t.TempDir()
	socketPath := filepath.Join(socketDir, "agent.sock")

	srv, err := agent.NewServer(backend, socketPath)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	go srv.Serve()

	// Wait for socket to appear.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		srv.Close()
		t.Fatalf("dial agent socket: %v", err)
	}

	client := sshagent.NewClient(conn)
	cleanup := func() {
		conn.Close()
		srv.Close()
	}
	return client, cleanup
}

func TestListKeys(t *testing.T) {
	key1, _ := newTestKey(t)
	key2, _ := newTestKey(t)
	key2.Comment = "test-key-2"

	backend := &mockBackend{keys: []*sshagent.Key{key1, key2}}
	client, cleanup := startTestAgent(t, backend)
	defer cleanup()

	keys, err := client.List()
	if err != nil {
		t.Fatalf("list keys: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
	if keys[0].Comment != "test-key" {
		t.Errorf("expected comment 'test-key', got %q", keys[0].Comment)
	}
	if keys[1].Comment != "test-key-2" {
		t.Errorf("expected comment 'test-key-2', got %q", keys[1].Comment)
	}
}

func TestListKeysEmpty(t *testing.T) {
	backend := &mockBackend{keys: []*sshagent.Key{}}
	client, cleanup := startTestAgent(t, backend)
	defer cleanup()

	keys, err := client.List()
	if err != nil {
		t.Fatalf("list keys: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("expected 0 keys, got %d", len(keys))
	}
}

func TestSignRequest(t *testing.T) {
	key1, signer := newTestKey(t)

	backend := &mockBackend{
		keys: []*sshagent.Key{key1},
		signFn: func(key ssh.PublicKey, data []byte, flags sshagent.SignatureFlags) (*ssh.Signature, error) {
			sig, err := signer.Sign(rand.Reader, data)
			if err != nil {
				return nil, err
			}
			return sig, nil
		},
	}
	client, cleanup := startTestAgent(t, backend)
	defer cleanup()

	pub, err := ssh.ParsePublicKey(key1.Blob)
	if err != nil {
		t.Fatalf("parse public key: %v", err)
	}

	data := []byte("data to sign")
	sig, err := client.Sign(pub, data)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if sig == nil {
		t.Fatal("signature is nil")
	}

	// Verify signature is valid.
	err = pub.Verify(data, sig)
	if err != nil {
		t.Fatalf("verify signature: %v", err)
	}
}

// TestSignRequestBackendError verifies that backend errors result in
// SSH_AGENT_FAILURE with no internal details leaked (FR-097).
func TestSignRequestBackendError(t *testing.T) {
	key1, _ := newTestKey(t)

	internalErr := errors.New("internal: /home/user/.config/nix-key/certs/device.pem: connection refused to 100.64.0.5:9876")
	backend := &mockBackend{
		keys:    []*sshagent.Key{key1},
		signErr: internalErr,
	}
	client, cleanup := startTestAgent(t, backend)
	defer cleanup()

	pub, err := ssh.ParsePublicKey(key1.Blob)
	if err != nil {
		t.Fatalf("parse public key: %v", err)
	}

	_, err = client.Sign(pub, []byte("test data"))
	if err == nil {
		t.Fatal("expected error from sign, got nil")
	}
	// The error message visible to the SSH client must NOT contain internal details.
	errMsg := err.Error()
	if contains(errMsg, "/home") || contains(errMsg, "100.64") || contains(errMsg, "connection refused") || contains(errMsg, ".pem") {
		t.Errorf("error message leaked internal details: %s", errMsg)
	}
}

// TestListKeysBackendError verifies that list errors result in
// SSH_AGENT_FAILURE with no internal details (FR-097).
func TestListKeysBackendError(t *testing.T) {
	internalErr := errors.New("internal: failed to reach device at 100.64.0.5: certificate expired")
	backend := &mockBackend{
		listErr: internalErr,
	}
	client, cleanup := startTestAgent(t, backend)
	defer cleanup()

	_, err := client.List()
	if err == nil {
		t.Fatal("expected error from list, got nil")
	}
	errMsg := err.Error()
	if contains(errMsg, "100.64") || contains(errMsg, "certificate expired") {
		t.Errorf("error message leaked internal details: %s", errMsg)
	}
}

func TestMultipleConcurrentConnections(t *testing.T) {
	key1, signer := newTestKey(t)

	backend := &mockBackend{
		keys: []*sshagent.Key{key1},
		signFn: func(key ssh.PublicKey, data []byte, flags sshagent.SignatureFlags) (*ssh.Signature, error) {
			return signer.Sign(rand.Reader, data)
		},
	}

	socketDir := t.TempDir()
	socketPath := filepath.Join(socketDir, "agent.sock")

	srv, err := agent.NewServer(backend, socketPath)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	go srv.Serve()
	defer srv.Close()

	// Wait for socket.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Spawn multiple concurrent clients.
	errCh := make(chan error, 5)
	for i := 0; i < 5; i++ {
		go func() {
			conn, err := net.Dial("unix", socketPath)
			if err != nil {
				errCh <- err
				return
			}
			defer conn.Close()

			c := sshagent.NewClient(conn)
			keys, err := c.List()
			if err != nil {
				errCh <- err
				return
			}
			if len(keys) != 1 {
				errCh <- errors.New("expected 1 key")
				return
			}
			errCh <- nil
		}()
	}

	for i := 0; i < 5; i++ {
		if err := <-errCh; err != nil {
			t.Errorf("concurrent client %d: %v", i, err)
		}
	}
}

func TestUnsupportedOperations(t *testing.T) {
	backend := &mockBackend{keys: []*sshagent.Key{}}
	client, cleanup := startTestAgent(t, backend)
	defer cleanup()

	// Add, Remove, RemoveAll, Lock, Unlock should all fail gracefully.
	err := client.Add(sshagent.AddedKey{})
	if err == nil {
		t.Error("expected error from Add")
	}

	err = client.RemoveAll()
	if err == nil {
		t.Error("expected error from RemoveAll")
	}

	err = client.Lock([]byte("pass"))
	if err == nil {
		t.Error("expected error from Lock")
	}

	err = client.Unlock([]byte("pass"))
	if err == nil {
		t.Error("expected error from Unlock")
	}
}

func TestServerClose(t *testing.T) {
	backend := &mockBackend{keys: []*sshagent.Key{}}
	socketDir := t.TempDir()
	socketPath := filepath.Join(socketDir, "agent.sock")

	srv, err := agent.NewServer(backend, socketPath)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	go srv.Serve()

	// Wait for socket.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	srv.Close()

	// After close, new connections should fail.
	time.Sleep(50 * time.Millisecond)
	_, err = net.Dial("unix", socketPath)
	if err == nil {
		t.Error("expected connection to fail after server close")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
