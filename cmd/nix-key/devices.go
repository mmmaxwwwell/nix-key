package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/phaedrus-raznikov/nix-key/internal/daemon"
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

	formatDevicesTable(os.Stdout, devices)
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

func formatDevicesTable(w io.Writer, devices []daemon.DeviceInfo) {
	if len(devices) == 0 {
		fmt.Fprintln(w, "No devices paired.")
		return
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tTAILSCALE IP\tCERT FINGERPRINT\tLAST SEEN\tSOURCE")

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

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			d.Name, ip, fp, lastSeen, d.Source)
	}
	tw.Flush()
}
