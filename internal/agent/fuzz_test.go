package agent

import (
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	sshagent "golang.org/x/crypto/ssh/agent"
)

// FuzzSSHAgentProtocol feeds arbitrary bytes into the SSH agent protocol parser.
// The agent should handle any input without panicking.
func FuzzSSHAgentProtocol(f *testing.F) {
	// Seed: SSH2_AGENTC_REQUEST_IDENTITIES (message type 11)
	f.Add([]byte{0x00, 0x00, 0x00, 0x01, 0x0b})
	// Seed: SSH2_AGENTC_SIGN_REQUEST (message type 13, minimal)
	f.Add([]byte{0x00, 0x00, 0x00, 0x01, 0x0d})
	// Seed: empty
	f.Add([]byte{})
	// Seed: single zero byte
	f.Add([]byte{0x00})

	f.Fuzz(func(t *testing.T, data []byte) {
		client, server := net.Pipe()
		// Prevent hangs from malformed length prefixes that cause
		// ServeAgent to block indefinitely or allocate excessive memory.
		deadline := time.Now().Add(2 * time.Second)
		client.SetDeadline(deadline)
		server.SetDeadline(deadline)
		defer client.Close()

		done := make(chan struct{})
		go func() {
			defer close(done)
			a := &sshAgent{backend: &fuzzBackend{}}
			_ = sshagent.ServeAgent(a, server)
			server.Close()
		}()

		// Write fuzz data then close to signal EOF
		_, _ = client.Write(data)
		client.Close()
		<-done
	})
}

// fuzzBackend is a minimal Backend for fuzz testing that returns empty results.
type fuzzBackend struct{}

func (fuzzBackend) List() ([]*sshagent.Key, error) {
	return []*sshagent.Key{}, nil
}

func (fuzzBackend) Sign(_ ssh.PublicKey, _ []byte, _ sshagent.SignatureFlags) (*ssh.Signature, error) {
	return nil, errUnsupported
}
