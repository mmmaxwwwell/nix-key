package main

import (
	"fmt"
	"io"

	"github.com/phaedrus-raznikov/nix-key/internal/daemon"
)

func runRevoke(controlSocket, deviceID string, w io.Writer) error {
	client := daemon.NewControlClient(controlSocket)
	resp, err := client.SendCommand(daemon.Request{
		Command:  "revoke-device",
		DeviceID: deviceID,
	})
	if err != nil {
		return fmt.Errorf("failed to query daemon: %w", err)
	}

	if resp.Status != "ok" {
		return fmt.Errorf("daemon error: %s", resp.Error)
	}

	fmt.Fprintf(w, "Device %q revoked successfully.\n", deviceID)
	return nil
}
