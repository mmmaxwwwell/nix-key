// backend.go implements the agent.Backend interface by forwarding
// key listing and signing operations to paired phone devices over gRPC.
package agent

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	sshagent "golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh"

	"github.com/phaedrus-raznikov/nix-key/internal/daemon"

	nixkeyv1 "github.com/phaedrus-raznikov/nix-key/gen/nixkey/v1"
)

// Dialer abstracts the establishment of a gRPC connection to a phone device.
// Production implementations will use mTLS over the Tailscale interface (FR-015).
type Dialer interface {
	// DialDevice connects to the given device and returns a gRPC client.
	// The returned cleanup function must be called to release resources.
	DialDevice(ctx context.Context, dev daemon.Device) (nixkeyv1.NixKeyAgentClient, func(), error)
}

// cachedKey maps an SSH key fingerprint to a device and its SSH agent key.
type cachedKey struct {
	deviceID string
	key      *sshagent.Key
}

// GRPCBackend implements agent.Backend by dialing phone devices over gRPC.
type GRPCBackend struct {
	registry          *daemon.Registry
	dialer            Dialer
	allowKeyListing   bool
	connectionTimeout time.Duration
	signTimeout       time.Duration

	// Logger for mTLS failure details (FR-E05). Nil disables logging.
	Logger *log.Logger

	mu       sync.RWMutex
	keyCache map[string]*cachedKey // SSH fingerprint -> cachedKey
}

// GRPCBackendConfig holds configuration for constructing a GRPCBackend.
type GRPCBackendConfig struct {
	Registry          *daemon.Registry
	Dialer            Dialer
	AllowKeyListing   bool
	ConnectionTimeout time.Duration
	SignTimeout       time.Duration
	Logger            *log.Logger
}

// NewGRPCBackend creates a new GRPCBackend.
func NewGRPCBackend(cfg GRPCBackendConfig) *GRPCBackend {
	if cfg.ConnectionTimeout == 0 {
		cfg.ConnectionTimeout = 10 * time.Second
	}
	if cfg.SignTimeout == 0 {
		cfg.SignTimeout = 30 * time.Second
	}
	return &GRPCBackend{
		registry:          cfg.Registry,
		dialer:            cfg.Dialer,
		allowKeyListing:   cfg.AllowKeyListing,
		connectionTimeout: cfg.ConnectionTimeout,
		signTimeout:       cfg.SignTimeout,
		Logger:            cfg.Logger,
		keyCache:          make(map[string]*cachedKey),
	}
}

// List returns all SSH keys from reachable devices.
// If allowKeyListing is false, returns empty without dialing (FR-005).
func (b *GRPCBackend) List() ([]*sshagent.Key, error) {
	if !b.allowKeyListing {
		return []*sshagent.Key{}, nil
	}

	devices := b.registry.ListReachable()
	if len(devices) == 0 {
		return []*sshagent.Key{}, nil
	}

	type deviceResult struct {
		deviceID string
		keys     []*nixkeyv1.SSHKey
		err      error
	}

	results := make(chan deviceResult, len(devices))

	for _, dev := range devices {
		go func(d daemon.Device) {
			ctx, cancel := context.WithTimeout(context.Background(), b.connectionTimeout)
			defer cancel()

			client, cleanup, err := b.dialer.DialDevice(ctx, d)
			if err != nil {
				b.logDialError(d, err)
				results <- deviceResult{deviceID: d.ID, err: err}
				return
			}
			defer cleanup()

			resp, err := client.ListKeys(ctx, &nixkeyv1.ListKeysRequest{})
			if err != nil {
				results <- deviceResult{deviceID: d.ID, err: err}
				return
			}
			results <- deviceResult{deviceID: d.ID, keys: resp.GetKeys()}
		}(dev)
	}

	var allKeys []*sshagent.Key
	newCache := make(map[string]*cachedKey)

	for range devices {
		r := <-results
		if r.err != nil {
			continue // FR-007: skip unreachable phones
		}
		for _, k := range r.keys {
			agentKey := &sshagent.Key{
				Format:  k.GetKeyType(),
				Blob:    k.GetPublicKeyBlob(),
				Comment: k.GetDisplayName(),
			}
			allKeys = append(allKeys, agentKey)
			newCache[k.GetFingerprint()] = &cachedKey{
				deviceID: r.deviceID,
				key:      agentKey,
			}
		}
	}

	b.mu.Lock()
	b.keyCache = newCache
	b.mu.Unlock()

	if allKeys == nil {
		allKeys = []*sshagent.Key{}
	}
	return allKeys, nil
}

// Sign forwards a sign request to the device that owns the key.
// The device is identified by the SSH key fingerprint from the cache.
func (b *GRPCBackend) Sign(key ssh.PublicKey, data []byte, flags sshagent.SignatureFlags) (*ssh.Signature, error) {
	fp := ssh.FingerprintSHA256(key)

	b.mu.RLock()
	cached, ok := b.keyCache[fp]
	b.mu.RUnlock()

	if !ok {
		// Key not in cache — refresh by listing all devices.
		if _, err := b.List(); err != nil {
			return nil, fmt.Errorf("refresh key cache: %w", err)
		}
		b.mu.RLock()
		cached, ok = b.keyCache[fp]
		b.mu.RUnlock()
		if !ok {
			return nil, fmt.Errorf("no device found for key %s", fp)
		}
	}

	dev, found := b.registry.Get(cached.deviceID)
	if !found {
		return nil, fmt.Errorf("device %s no longer in registry", cached.deviceID)
	}

	// Dial with connectionTimeout.
	dialCtx, dialCancel := context.WithTimeout(context.Background(), b.connectionTimeout)
	defer dialCancel()

	client, cleanup, err := b.dialer.DialDevice(dialCtx, dev)
	if err != nil {
		b.logDialError(dev, err)
		return nil, fmt.Errorf("dial device %s: %w", dev.Name, err)
	}
	defer cleanup()

	// Sign with signTimeout.
	signCtx, signCancel := context.WithTimeout(context.Background(), b.signTimeout)
	defer signCancel()

	resp, err := client.Sign(signCtx, &nixkeyv1.SignRequest{
		KeyFingerprint: fp,
		Data:           data,
		Flags:          uint32(flags),
	})
	if err != nil {
		return nil, fmt.Errorf("sign RPC to %s: %w", dev.Name, err)
	}

	// Parse the SSH signature from the raw bytes.
	sig := &ssh.Signature{}
	if err := ssh.Unmarshal(resp.GetSignature(), sig); err != nil {
		// If unmarshal fails, try treating as raw signature with key type format.
		sig = &ssh.Signature{
			Format: key.Type(),
			Blob:   resp.GetSignature(),
		}
	}

	// Update lastSeen on successful sign.
	b.registry.UpdateLastSeen(dev.ID, time.Now())

	return sig, nil
}

// logDialError logs mTLS connection failure details (FR-E05).
func (b *GRPCBackend) logDialError(dev daemon.Device, err error) {
	if b.Logger != nil {
		b.Logger.Printf("mTLS dial failed: device=%s ip=%s port=%d fingerprint=%s error=%v",
			dev.Name, dev.TailscaleIP, dev.ListenPort, dev.CertFingerprint, err)
	}
}
