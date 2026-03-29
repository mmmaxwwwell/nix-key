package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/phaedrus-raznikov/nix-key/internal/daemon"
	"github.com/phaedrus-raznikov/nix-key/internal/mtls"

	nixkeyv1 "github.com/phaedrus-raznikov/nix-key/gen/nixkey/v1"
)

// runTestDevice resolves a device from the daemon registry, dials it via mTLS,
// calls the Ping RPC, and reports success with round-trip latency or a specific error.
func runTestDevice(controlSocket, deviceID, ageIdentityPath string, timeout time.Duration, w io.Writer) error {
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	// Step 1: Query daemon for device info.
	client := daemon.NewControlClient(controlSocket)
	resp, err := client.SendCommand(daemon.Request{
		Command:  "get-device",
		DeviceID: deviceID,
	})
	if err != nil {
		return fmt.Errorf("failed to query daemon: %w", err)
	}

	dev, err := parseFullDeviceInfo(resp)
	if err != nil {
		return err
	}

	// Step 2: Validate device has connectivity info.
	if dev.TailscaleIP == "" || dev.ListenPort == 0 {
		return fmt.Errorf("device %q is unreachable: no Tailscale IP or listen port configured", dev.Name)
	}

	if dev.ClientCertPath == "" || dev.ClientKeyPath == "" {
		return fmt.Errorf("device %q has no client cert/key configured", dev.Name)
	}

	addr := fmt.Sprintf("%s:%d", dev.TailscaleIP, dev.ListenPort)
	fmt.Fprintf(w, "Testing device %q at %s...\n", dev.Name, addr)

	// Step 3: mTLS dial to phone.
	conn, err := mtls.DialMTLS(addr, dev.ClientCertPath, dev.ClientKeyPath, dev.CertFingerprint, ageIdentityPath)
	if err != nil {
		return classifyDialError(err, dev.Name)
	}
	defer conn.Close()

	// Step 4: Call Ping RPC.
	grpcClient := nixkeyv1.NewNixKeyAgentClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	_, err = grpcClient.Ping(ctx, &nixkeyv1.PingRequest{})
	elapsed := time.Since(start)

	if err != nil {
		return classifyRPCError(err, dev.Name)
	}

	fmt.Fprintf(w, "OK: %s responded in %s\n", dev.Name, formatLatency(elapsed))
	return nil
}

// parseFullDeviceInfo extracts FullDeviceInfo from a control socket response.
func parseFullDeviceInfo(resp *daemon.Response) (*daemon.FullDeviceInfo, error) {
	if resp.Status != "ok" {
		return nil, fmt.Errorf("daemon error: %s", resp.Error)
	}

	data, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal response data: %w", err)
	}

	var dev daemon.FullDeviceInfo
	if err := json.Unmarshal(data, &dev); err != nil {
		return nil, fmt.Errorf("parse device info: %w", err)
	}
	return &dev, nil
}

// classifyDialError translates mTLS dial errors into user-friendly messages.
func classifyDialError(err error, deviceName string) error {
	errStr := err.Error()

	if strings.Contains(errStr, "no such file or directory") {
		return fmt.Errorf("device %q: cert files not found (were they revoked?): %w", deviceName, err)
	}

	if strings.Contains(errStr, "certificate") || strings.Contains(errStr, "tls") {
		return fmt.Errorf("device %q: cert mismatch or TLS error: %w", deviceName, err)
	}

	return fmt.Errorf("device %q: connection failed: %w", deviceName, err)
}

// classifyRPCError translates gRPC Ping errors into user-friendly messages.
func classifyRPCError(err error, deviceName string) error {
	errStr := err.Error()

	if strings.Contains(errStr, "DeadlineExceeded") || strings.Contains(errStr, "context deadline exceeded") {
		return fmt.Errorf("device %q: timeout waiting for Ping response", deviceName)
	}

	if strings.Contains(errStr, "Unavailable") || strings.Contains(errStr, "connection refused") {
		return fmt.Errorf("device %q: unreachable (connection refused or dropped)", deviceName)
	}

	if strings.Contains(errStr, "certificate") || strings.Contains(errStr, "fingerprint") {
		return fmt.Errorf("device %q: cert mismatch during Ping: %w", deviceName, err)
	}

	return fmt.Errorf("device %q: Ping failed: %w", deviceName, err)
}

// formatLatency formats a duration as a human-readable latency string.
func formatLatency(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	return fmt.Sprintf("%.1fms", float64(d.Microseconds())/1000.0)
}
