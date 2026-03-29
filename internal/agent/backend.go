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
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/phaedrus-raznikov/nix-key/internal/daemon"

	nixkeyv1 "github.com/phaedrus-raznikov/nix-key/gen/nixkey/v1"
)

// Dialer abstracts the establishment of a gRPC connection to a phone device.
// Production implementations will use mTLS over the Tailscale interface (FR-015).
// The context carries OTEL trace information for W3C traceparent propagation
// via otelgrpc interceptors on the gRPC connection.
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
	tracer            trace.Tracer

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
	Tracer            trace.Tracer
}

// NewGRPCBackend creates a new GRPCBackend.
func NewGRPCBackend(cfg GRPCBackendConfig) *GRPCBackend {
	if cfg.ConnectionTimeout == 0 {
		cfg.ConnectionTimeout = 10 * time.Second
	}
	if cfg.SignTimeout == 0 {
		cfg.SignTimeout = 30 * time.Second
	}
	tracer := cfg.Tracer
	if tracer == nil {
		tracer = noop.NewTracerProvider().Tracer("nix-key")
	}
	return &GRPCBackend{
		registry:          cfg.Registry,
		dialer:            cfg.Dialer,
		allowKeyListing:   cfg.AllowKeyListing,
		connectionTimeout: cfg.ConnectionTimeout,
		signTimeout:       cfg.SignTimeout,
		tracer:            tracer,
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
	ctx, span := b.tracer.Start(context.Background(), "ssh-sign-request",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	fp := ssh.FingerprintSHA256(key)
	span.SetAttributes(attribute.String("ssh.key.fingerprint", fp))

	// Device lookup phase.
	_, lookupSpan := b.tracer.Start(ctx, "device-lookup")
	b.mu.RLock()
	cached, ok := b.keyCache[fp]
	b.mu.RUnlock()

	if !ok {
		// Key not in cache — refresh by listing all devices.
		if _, err := b.List(); err != nil {
			lookupSpan.RecordError(err)
			lookupSpan.SetStatus(codes.Error, "cache refresh failed")
			lookupSpan.End()
			return nil, fmt.Errorf("refresh key cache: %w", err)
		}
		b.mu.RLock()
		cached, ok = b.keyCache[fp]
		b.mu.RUnlock()
		if !ok {
			err := fmt.Errorf("no device found for key %s", fp)
			lookupSpan.RecordError(err)
			lookupSpan.SetStatus(codes.Error, "key not found")
			lookupSpan.End()
			span.RecordError(err)
			span.SetStatus(codes.Error, "device lookup failed")
			return nil, err
		}
	}

	dev, found := b.registry.Get(cached.deviceID)
	if !found {
		err := fmt.Errorf("device %s no longer in registry", cached.deviceID)
		lookupSpan.RecordError(err)
		lookupSpan.SetStatus(codes.Error, "device removed")
		lookupSpan.End()
		span.RecordError(err)
		span.SetStatus(codes.Error, "device lookup failed")
		return nil, err
	}
	lookupSpan.SetAttributes(
		attribute.String("device.id", dev.ID),
		attribute.String("device.name", dev.Name),
	)
	lookupSpan.End()

	span.SetAttributes(
		attribute.String("device.id", dev.ID),
		attribute.String("device.name", dev.Name),
	)

	// mTLS connect phase.
	dialCtx, dialCancel := context.WithTimeout(ctx, b.connectionTimeout)
	defer dialCancel()

	dialCtx, connectSpan := b.tracer.Start(dialCtx, "mtls-connect")
	connectSpan.SetAttributes(
		attribute.String("device.ip", dev.TailscaleIP),
		attribute.Int("device.port", dev.ListenPort),
	)

	client, cleanup, err := b.dialer.DialDevice(dialCtx, dev)
	if err != nil {
		b.logDialError(dev, err)
		connectSpan.RecordError(err)
		connectSpan.SetStatus(codes.Error, "dial failed")
		connectSpan.End()
		span.RecordError(err)
		span.SetStatus(codes.Error, "connection failed")
		return nil, fmt.Errorf("dial device %s: %w", dev.Name, err)
	}
	defer cleanup()
	connectSpan.End()

	// Sign RPC phase — context carries trace for W3C traceparent propagation via otelgrpc.
	signCtx, signCancel := context.WithTimeout(ctx, b.signTimeout)
	defer signCancel()

	resp, err := client.Sign(signCtx, &nixkeyv1.SignRequest{
		KeyFingerprint: fp,
		Data:           data,
		Flags:          uint32(flags),
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "sign RPC failed")
		return nil, fmt.Errorf("sign RPC to %s: %w", dev.Name, err)
	}

	// Return signature phase.
	_, returnSpan := b.tracer.Start(ctx, "return-signature")
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
	returnSpan.End()

	return sig, nil
}

// logDialError logs mTLS connection failure details (FR-E05).
func (b *GRPCBackend) logDialError(dev daemon.Device, err error) {
	if b.Logger != nil {
		b.Logger.Printf("mTLS dial failed: device=%s ip=%s port=%d fingerprint=%s error=%v",
			dev.Name, dev.TailscaleIP, dev.ListenPort, dev.CertFingerprint, err)
	}
}
