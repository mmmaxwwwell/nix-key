// Package agent implements an SSH agent protocol handler that delegates
// key listing and signing operations to a pluggable Backend.
//
// It listens on a Unix domain socket and serves the SSH agent protocol
// (SSH2_AGENTC_REQUEST_IDENTITIES and SSH2_AGENTC_SIGN_REQUEST).
// All errors returned to SSH clients are sanitized per FR-097.
package agent

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"

	sshagent "golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh"
)

// errUnsupported is returned for SSH agent operations that nix-key does not support.
var errUnsupported = errors.New("operation not supported")

// errAgentFailure is the sanitized error returned to SSH clients.
// It intentionally contains no internal details (FR-097).
var errAgentFailure = errors.New("agent failure")

// Backend defines the interface the SSH agent delegates to for key operations.
type Backend interface {
	// List returns the SSH keys available for signing.
	List() ([]*sshagent.Key, error)

	// Sign signs data with the key matching the given public key.
	Sign(key ssh.PublicKey, data []byte, flags sshagent.SignatureFlags) (*ssh.Signature, error)
}

// sshAgent implements golang.org/x/crypto/ssh/agent.ExtendedAgent by
// wrapping a Backend. Unsupported operations return errors. All backend
// errors are sanitized before being returned to the SSH client.
type sshAgent struct {
	backend Backend
}

func (a *sshAgent) List() ([]*sshagent.Key, error) {
	keys, err := a.backend.List()
	if err != nil {
		return nil, errAgentFailure
	}
	return keys, nil
}

func (a *sshAgent) Sign(key ssh.PublicKey, data []byte) (*ssh.Signature, error) {
	sig, err := a.backend.Sign(key, data, 0)
	if err != nil {
		return nil, errAgentFailure
	}
	return sig, nil
}

func (a *sshAgent) SignWithFlags(key ssh.PublicKey, data []byte, flags sshagent.SignatureFlags) (*ssh.Signature, error) {
	sig, err := a.backend.Sign(key, data, flags)
	if err != nil {
		return nil, errAgentFailure
	}
	return sig, nil
}

func (a *sshAgent) Extension(extensionType string, contents []byte) ([]byte, error) {
	return nil, sshagent.ErrExtensionUnsupported
}

func (a *sshAgent) Add(key sshagent.AddedKey) error    { return errUnsupported }
func (a *sshAgent) Remove(key ssh.PublicKey) error      { return errUnsupported }
func (a *sshAgent) RemoveAll() error                    { return errUnsupported }
func (a *sshAgent) Lock(passphrase []byte) error        { return errUnsupported }
func (a *sshAgent) Unlock(passphrase []byte) error      { return errUnsupported }
func (a *sshAgent) Signers() ([]ssh.Signer, error)      { return nil, errUnsupported }

// Server listens on a Unix socket and serves the SSH agent protocol.
type Server struct {
	backend  Backend
	listener net.Listener
	mu       sync.Mutex
	closed   bool
	wg       sync.WaitGroup
}

// NewServer creates a new SSH agent server listening at socketPath.
// The parent directory of socketPath is created if it does not exist.
func NewServer(backend Backend, socketPath string) (*Server, error) {
	if err := os.MkdirAll(filepath.Dir(socketPath), 0700); err != nil {
		return nil, fmt.Errorf("create socket directory: %w", err)
	}

	// Remove stale socket file if it exists.
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove stale socket: %w", err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", socketPath, err)
	}

	// Restrict socket permissions to owner only.
	if err := os.Chmod(socketPath, 0600); err != nil {
		listener.Close()
		return nil, fmt.Errorf("chmod socket: %w", err)
	}

	return &Server{
		backend:  backend,
		listener: listener,
	}, nil
}

// Serve accepts connections and serves the SSH agent protocol.
// It blocks until the server is closed. Each connection is handled
// in a separate goroutine.
func (s *Server) Serve() error {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return nil
			}
			continue
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer conn.Close()
			a := &sshAgent{backend: s.backend}
			sshagent.ServeAgent(a, conn)
		}()
	}
}

// Close stops accepting new connections, closes the listener, and
// waits for in-flight connections to finish.
func (s *Server) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()

	err := s.listener.Close()
	s.wg.Wait()
	return err
}

// SocketPath returns the path of the Unix socket the server is listening on.
func (s *Server) SocketPath() string {
	return s.listener.Addr().String()
}
