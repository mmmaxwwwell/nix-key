package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"text/tabwriter"
	"time"

	nixkeyv1 "github.com/phaedrus-raznikov/nix-key/gen/nixkey/v1"
	"github.com/phaedrus-raznikov/nix-key/internal/daemon"
	"github.com/phaedrus-raznikov/nix-key/internal/mtls"
)

// defaultControlSocket returns the default control socket path based on
// XDG_RUNTIME_DIR (typically /run/user/<uid>).
func defaultControlSocket() string {
	runDir := os.Getenv("XDG_RUNTIME_DIR")
	if runDir == "" {
		runDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	return filepath.Join(runDir, "nix-key", "control.sock")
}

func runDevices(controlSocket string) error {
	client := daemon.NewControlClient(controlSocket)
	resp, err := client.SendCommand(daemon.Request{Command: "list-devices"})
	if err != nil {
		return fmt.Errorf("failed to query daemon: %w", err)
	}

	devices, err := parseDeviceInfos(resp)
	if err != nil {
		return err
	}

	statuses := probeDeviceStatuses(devices)
	formatDevicesTable(os.Stdout, devices, statuses)
	return nil
}

// parseDeviceInfos extracts []DeviceInfo from a control socket response.
// The Data field arrives as an interface{} which needs re-marshaling.
func parseDeviceInfos(resp *daemon.Response) ([]daemon.DeviceInfo, error) {
	if resp.Status != "ok" {
		return nil, fmt.Errorf("daemon error: %s", resp.Error)
	}

	data, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal response data: %w", err)
	}

	var devices []daemon.DeviceInfo
	if err := json.Unmarshal(data, &devices); err != nil {
		return nil, fmt.Errorf("parse device list: %w", err)
	}
	return devices, nil
}

// probeDeviceStatuses performs concurrent mTLS Ping RPCs to each device
// and returns a map of device ID -> status string ("online", "offline", "unknown").
func probeDeviceStatuses(devices []daemon.DeviceInfo) map[string]string {
	statuses := make(map[string]string, len(devices))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, d := range devices {
		d := d
		wg.Add(1)
		go func() {
			defer wg.Done()
			status := pingDevice(d)
			mu.Lock()
			statuses[d.ID] = status
			mu.Unlock()
		}()
	}

	wg.Wait()
	return statuses
}

// pingDevice attempts an mTLS Ping RPC to a single device with a 2s timeout.
// Returns "online", "offline", or "unknown".
func pingDevice(d daemon.DeviceInfo) string {
	if d.TailscaleIP == "" || d.ListenPort == 0 {
		return "unknown"
	}
	if d.ClientCertPath == "" || d.ClientKeyPath == "" {
		return "unknown"
	}

	addr := fmt.Sprintf("%s:%d", d.TailscaleIP, d.ListenPort)

	conn, err := mtls.DialMTLS(addr, d.ClientCertPath, d.ClientKeyPath, d.CertFingerprint, "")
	if err != nil {
		return "offline"
	}
	defer func() { _ = conn.Close() }()

	grpcClient := nixkeyv1.NewNixKeyAgentClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = grpcClient.Ping(ctx, &nixkeyv1.PingRequest{})
	if err != nil {
		return "offline"
	}
	return "online"
}

// truncateFingerprint returns the first n characters of a cert fingerprint
// for display. If the fingerprint includes a "SHA256:" prefix, that prefix
// plus n hex characters are returned.
func truncateFingerprint(fp string, n int) string {
	if len(fp) <= n {
		return fp
	}
	const prefix = "SHA256:"
	if len(fp) > len(prefix) && fp[:len(prefix)] == prefix {
		hex := fp[len(prefix):]
		if len(hex) > n {
			hex = hex[:n]
		}
		return prefix + hex
	}
	return fp[:n]
}

func formatDevicesTable(w io.Writer, devices []daemon.DeviceInfo, statuses map[string]string) {
	if len(devices) == 0 {
		fmt.Fprintln(w, "No devices paired.")
		return
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tTAILSCALE IP\tCERT FINGERPRINT\tLAST SEEN\tSTATUS\tSOURCE")

	for _, d := range devices {
		ip := d.TailscaleIP
		if ip == "" {
			ip = "-"
		}

		fp := truncateFingerprint(d.CertFingerprint, 8)

		lastSeen := "never"
		if d.LastSeen != nil {
			lastSeen = d.LastSeen.Format("2006-01-02 15:04:05")
		}

		status := "unknown"
		if s, ok := statuses[d.ID]; ok {
			status = s
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			d.Name, ip, fp, lastSeen, status, d.Source)
	}
	_ = tw.Flush()
}
