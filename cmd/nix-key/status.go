package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/phaedrus-raznikov/nix-key/internal/daemon"
)

func runStatus(controlSocket string, w io.Writer) error {
	client := daemon.NewControlClient(controlSocket)
	resp, err := client.SendCommand(daemon.Request{Command: "get-status"})
	if err != nil {
		return fmt.Errorf("daemon is not running (failed to connect: %w)", err)
	}

	status, err := parseStatusInfo(resp)
	if err != nil {
		return err
	}

	formatStatus(w, status)
	return nil
}

// parseStatusInfo extracts StatusInfo from a control socket response.
func parseStatusInfo(resp *daemon.Response) (*daemon.StatusInfo, error) {
	if resp.Status != "ok" {
		return nil, fmt.Errorf("daemon error: %s", resp.Error)
	}

	data, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal response data: %w", err)
	}

	var status daemon.StatusInfo
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, fmt.Errorf("parse status info: %w", err)
	}
	return &status, nil
}

func formatStatus(w io.Writer, status *daemon.StatusInfo) {
	state := "stopped"
	if status.Running {
		state = "running"
	}

	fmt.Fprintf(w, "Daemon:           %s\n", state)
	fmt.Fprintf(w, "Socket:           %s\n", status.SocketPath)
	fmt.Fprintf(w, "Connected devices: %d\n", status.DeviceCount)
	fmt.Fprintf(w, "Available keys:   %d\n", status.KeyCount)

	if len(status.CertWarnings) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Certificate warnings:")
		for _, cw := range status.CertWarnings {
			name := cw.DeviceName
			if name == "" {
				name = cw.DeviceID
			}
			if cw.DaysLeft == 0 {
				fmt.Fprintf(w, "  WARNING: %s cert for %q has expired\n", cw.CertType, name)
			} else {
				fmt.Fprintf(w, "  WARNING: %s cert for %q expires in %d days\n", cw.CertType, name, cw.DaysLeft)
			}
		}
	}
}

// runStatusOrNotRunning handles the case where the daemon may not be running.
// If the control socket cannot be reached, it prints a not-running message
// instead of returning an error.
func runStatusOrNotRunning(controlSocket string) error {
	client := daemon.NewControlClient(controlSocket)
	resp, err := client.SendCommand(daemon.Request{Command: "get-status"})
	if err != nil {
		fmt.Fprintf(os.Stdout, "Daemon:           stopped\n")
		fmt.Fprintf(os.Stdout, "Socket:           %s (not reachable)\n", controlSocket)
		return nil
	}

	status, err := parseStatusInfo(resp)
	if err != nil {
		return err
	}

	formatStatus(os.Stdout, status)
	return nil
}
