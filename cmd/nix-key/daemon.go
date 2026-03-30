package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/phaedrus-raznikov/nix-key/internal/agent"
	"github.com/phaedrus-raznikov/nix-key/internal/config"
	"github.com/phaedrus-raznikov/nix-key/internal/daemon"
	"github.com/phaedrus-raznikov/nix-key/internal/mtls"
	"github.com/phaedrus-raznikov/nix-key/internal/tracing"

	nixkeyv1 "github.com/phaedrus-raznikov/nix-key/gen/nixkey/v1"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
)

// defaultStateDir returns the default state directory based on
// XDG_STATE_HOME (typically ~/.local/state).
func defaultStateDir() string {
	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		home, _ := os.UserHomeDir()
		stateDir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(stateDir, "nix-key")
}

// productionDialer implements agent.Dialer for real connections.
// It uses mTLS when device cert paths are set, plain gRPC otherwise.
type productionDialer struct {
	ageKeyFile string
	tracer     *tracing.Provider
}

func (d *productionDialer) DialDevice(ctx context.Context, dev daemon.Device) (nixkeyv1.NixKeyAgentClient, func(), error) {
	addr := fmt.Sprintf("%s:%d", dev.TailscaleIP, dev.ListenPort)

	var conn *grpc.ClientConn
	var err error

	if dev.ClientCertPath != "" && dev.ClientKeyPath != "" && dev.CertPath != "" {
		// mTLS connection
		var extraOpts []grpc.DialOption
		if d.tracer != nil {
			extraOpts = append(extraOpts,
				grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
			)
		}
		conn, err = mtls.DialMTLS(addr, dev.ClientCertPath, dev.ClientKeyPath, dev.CertFingerprint, d.ageKeyFile, extraOpts...)
	} else {
		// Plain gRPC (no mTLS) — used when device has no cert paths (e.g., phonesim in tests)
		opts := []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		}
		if d.tracer != nil {
			opts = append(opts, grpc.WithStatsHandler(otelgrpc.NewClientHandler()))
		}
		conn, err = grpc.NewClient(addr, opts...)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("dialing %s: %w", addr, err)
	}

	client := nixkeyv1.NewNixKeyAgentClient(conn)
	cleanup := func() { _ = conn.Close() }
	return client, cleanup, nil
}

func runDaemon(configPath string) error {
	// Load config
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize tracing
	tp, err := tracing.Init(ctx, cfg.OtelEndpoint)
	if err != nil {
		return fmt.Errorf("initializing tracing: %w", err)
	}

	// Set up shutdown manager
	sm := daemon.NewShutdownManager(30 * time.Second)
	sm.RegisterHook("tracing", func(ctx context.Context) error {
		return tp.Shutdown(ctx)
	})

	// Load devices
	stateDir := defaultStateDir()
	devicesPath := filepath.Join(stateDir, "devices.json")
	runtimeDevices, err := daemon.LoadDevicesFromJSON(devicesPath)
	if err != nil {
		return fmt.Errorf("loading devices: %w", err)
	}

	registry := daemon.NewRegistry()
	registry.Merge(nil, runtimeDevices)

	// Create dialer
	dialer := &productionDialer{
		ageKeyFile: cfg.AgeKeyFile,
		tracer:     tp,
	}

	// Create backend
	logger := log.New(os.Stderr, "", 0)
	backend := agent.NewGRPCBackend(agent.GRPCBackendConfig{
		Registry:          registry,
		Dialer:            dialer,
		AllowKeyListing:   cfg.AllowKeyListing,
		ConnectionTimeout: time.Duration(cfg.ConnectionTimeout) * time.Second,
		SignTimeout:       time.Duration(cfg.SignTimeout) * time.Second,
		Logger:            logger,
		Tracer:            tp.Tracer(),
	})

	// Start SSH agent server
	agentServer, err := agent.NewServer(backend, cfg.SocketPath)
	if err != nil {
		return fmt.Errorf("creating agent server: %w", err)
	}
	sm.RegisterHook("agent", func(_ context.Context) error {
		return agentServer.Close()
	})
	go func() { _ = agentServer.Serve() }()

	// Start control server
	controlServer := daemon.NewControlServer(daemon.ControlServerConfig{
		SocketPath:  cfg.ControlSocketPath,
		Registry:    registry,
		DevicesPath: devicesPath,
		KeyLister:   func() []daemon.KeyInfo { return nil },
	})
	if err := controlServer.Start(); err != nil {
		return fmt.Errorf("starting control server: %w", err)
	}
	sm.RegisterHook("control", func(_ context.Context) error {
		controlServer.Stop()
		return nil
	})

	fmt.Fprintf(os.Stderr, "nix-key daemon started, socket=%s\n", cfg.SocketPath)

	// Block until signal
	return sm.Run(ctx)
}
