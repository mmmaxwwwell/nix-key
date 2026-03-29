// Command phonesim is a phone simulator for E2E testing.
// It runs a gRPC NixKeyAgent server on a Tailscale network using tsnet,
// with in-memory Ed25519 and ECDSA keys and configurable behavior.
package main

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/phaedrus-raznikov/nix-key/pkg/phoneserver"
	"golang.org/x/crypto/ssh"
	"tailscale.com/tsnet"
)

func main() {
	var (
		tsAuthKey    string
		listenPort   int
		denyList     bool
		signDelay    time.Duration
		denySigning  bool
		hostname     string
		stateDir     string
		plainListen  string
	)

	flag.StringVar(&tsAuthKey, "ts-auth-key", "", "Tailscale auth key (required unless -plain-listen is set)")
	flag.IntVar(&listenPort, "port", 50051, "gRPC listen port on Tailscale interface")
	flag.BoolVar(&denyList, "deny-list", false, "deny key listing requests (return empty list)")
	flag.DurationVar(&signDelay, "sign-delay", 0, "artificial delay before signing (for timeout testing)")
	flag.BoolVar(&denySigning, "deny-sign", false, "deny all sign requests")
	flag.StringVar(&hostname, "hostname", "phonesim", "Tailscale hostname")
	flag.StringVar(&stateDir, "state-dir", "", "tsnet state directory (default: auto-managed temp dir)")
	flag.StringVar(&plainListen, "plain-listen", "", "if set, listen on this TCP address instead of Tailscale (e.g., 127.0.0.1:0)")
	flag.Parse()

	// Generate in-memory test keys.
	ks, err := newMemKeyStore()
	if err != nil {
		log.Fatalf("failed to create key store: %v", err)
	}

	// Build the confirmer based on flags.
	conf := &simConfirmer{
		deny:  denySigning,
		delay: signDelay,
	}

	// Wrap the key store if deny-list mode is enabled.
	var store phoneserver.KeyStore = ks
	if denyList {
		store = &denyListKeyStore{inner: ks}
	}

	srv := phoneserver.NewServer(store, conf)

	var lis net.Listener

	if plainListen != "" {
		// Plain TCP mode (no Tailscale) for simpler testing.
		lis, err = net.Listen("tcp", plainListen)
		if err != nil {
			log.Fatalf("plain listen: %v", err)
		}
		log.Printf("phonesim: listening on %s (plain TCP)", lis.Addr())
	} else {
		// Tailscale mode via tsnet.
		if tsAuthKey == "" {
			fmt.Fprintln(os.Stderr, "error: -ts-auth-key is required when not using -plain-listen")
			flag.Usage()
			os.Exit(1)
		}

		tsServer := &tsnet.Server{
			Hostname:  hostname,
			AuthKey:   tsAuthKey,
			Ephemeral: true,
		}
		if stateDir != "" {
			tsServer.Dir = stateDir
		}

		defer tsServer.Close()

		lis, err = tsServer.Listen("tcp", fmt.Sprintf(":%d", listenPort))
		if err != nil {
			log.Fatalf("tsnet listen: %v", err)
		}
		log.Printf("phonesim: listening on %s (Tailscale)", lis.Addr())
	}

	// Start serving in a goroutine so we can handle signals.
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(lis)
	}()

	// Wait for signal or server error.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	select {
	case sig := <-sigCh:
		log.Printf("phonesim: received %v, shutting down", sig)
		srv.Stop()
	case err := <-errCh:
		if err != nil {
			log.Fatalf("phonesim: server error: %v", err)
		}
	}
}

// memKeyStore is an in-memory KeyStore with pre-generated Ed25519 and ECDSA keys.
type memKeyStore struct {
	keys    []*phoneserver.Key
	signers map[string]interface{} // fingerprint → crypto.Signer
}

func newMemKeyStore() (*memKeyStore, error) {
	ks := &memKeyStore{
		signers: make(map[string]interface{}),
	}

	// Generate Ed25519 key.
	edPub, edPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ed25519: %w", err)
	}
	edSSH, err := ssh.NewPublicKey(edPub)
	if err != nil {
		return nil, fmt.Errorf("ssh ed25519 pubkey: %w", err)
	}
	edFP := ssh.FingerprintSHA256(edSSH)
	ks.keys = append(ks.keys, &phoneserver.Key{
		PublicKeyBlob: edSSH.Marshal(),
		KeyType:       edSSH.Type(),
		DisplayName:   "phonesim-ed25519",
		Fingerprint:   edFP,
	})
	ks.signers[edFP] = edPriv

	// Generate ECDSA P-256 key.
	ecPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ecdsa: %w", err)
	}
	ecSSH, err := ssh.NewPublicKey(&ecPriv.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("ssh ecdsa pubkey: %w", err)
	}
	ecFP := ssh.FingerprintSHA256(ecSSH)
	ks.keys = append(ks.keys, &phoneserver.Key{
		PublicKeyBlob: ecSSH.Marshal(),
		KeyType:       ecSSH.Type(),
		DisplayName:   "phonesim-ecdsa",
		Fingerprint:   ecFP,
	})
	ks.signers[ecFP] = ecPriv

	return ks, nil
}

func (m *memKeyStore) ListKeys() (*phoneserver.KeyList, error) {
	kl := phoneserver.NewKeyList()
	for _, k := range m.keys {
		kl.Add(k)
	}
	return kl, nil
}

func (m *memKeyStore) Sign(fingerprint string, data []byte, _ int32) ([]byte, error) {
	signer, ok := m.signers[fingerprint]
	if !ok {
		return nil, fmt.Errorf("key not found: %s", fingerprint)
	}

	switch s := signer.(type) {
	case ed25519.PrivateKey:
		sig := ed25519.Sign(s, data)
		// Return in SSH wire format.
		sshSig := &ssh.Signature{
			Format: "ssh-ed25519",
			Blob:   sig,
		}
		return ssh.Marshal(sshSig), nil

	case *ecdsa.PrivateKey:
		hash := sha256.Sum256(data)
		sigBytes, err := ecdsa.SignASN1(rand.Reader, s, hash[:])
		if err != nil {
			return nil, fmt.Errorf("ecdsa sign: %w", err)
		}
		sshSig := &ssh.Signature{
			Format: "ecdsa-sha2-nistp256",
			Blob:   sigBytes,
		}
		return ssh.Marshal(sshSig), nil

	default:
		return nil, fmt.Errorf("unsupported signer type: %T", s)
	}
}

// simConfirmer is a configurable Confirmer for testing scenarios.
type simConfirmer struct {
	deny  bool
	delay time.Duration
}

func (c *simConfirmer) RequestConfirmation(_, _, _ string) (bool, error) {
	if c.delay > 0 {
		time.Sleep(c.delay)
	}
	if c.deny {
		return false, nil
	}
	return true, nil
}

// denyListKeyStore wraps a KeyStore and returns an empty key list.
type denyListKeyStore struct {
	inner phoneserver.KeyStore
}

func (d *denyListKeyStore) ListKeys() (*phoneserver.KeyList, error) {
	return phoneserver.NewKeyList(), nil
}

func (d *denyListKeyStore) Sign(fingerprint string, data []byte, flags int32) ([]byte, error) {
	return d.inner.Sign(fingerprint, data, flags)
}
