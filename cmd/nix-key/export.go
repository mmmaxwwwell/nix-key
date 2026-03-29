package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/phaedrus-raznikov/nix-key/internal/daemon"
)

func runExport(controlSocket string, keyID string, w io.Writer) error {
	client := daemon.NewControlClient(controlSocket)
	resp, err := client.SendCommand(daemon.Request{Command: "get-keys"})
	if err != nil {
		return fmt.Errorf("failed to query daemon: %w", err)
	}

	keys, err := parseKeyInfos(resp)
	if err != nil {
		return err
	}

	match, err := findKeyByPrefix(keys, keyID)
	if err != nil {
		return err
	}

	fmt.Fprintln(w, match.PublicKey)
	return nil
}

// parseKeyInfos extracts []KeyInfo from a control socket response.
func parseKeyInfos(resp *daemon.Response) ([]daemon.KeyInfo, error) {
	if resp.Status != "ok" {
		return nil, fmt.Errorf("daemon error: %s", resp.Error)
	}

	data, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal response data: %w", err)
	}

	var keys []daemon.KeyInfo
	if err := json.Unmarshal(data, &keys); err != nil {
		return nil, fmt.Errorf("parse key list: %w", err)
	}
	return keys, nil
}

// findKeyByPrefix finds a key matching the given SHA256 fingerprint or unique
// prefix. Returns an error if no key matches or if the prefix is ambiguous.
func findKeyByPrefix(keys []daemon.KeyInfo, keyID string) (*daemon.KeyInfo, error) {
	// Normalize: if user provides bare hash without "SHA256:" prefix,
	// prepend it for matching.
	normalizedID := keyID
	if !strings.HasPrefix(keyID, "SHA256:") {
		normalizedID = "SHA256:" + keyID
	}

	var matches []daemon.KeyInfo
	for _, k := range keys {
		if k.Fingerprint == normalizedID {
			// Exact match — return immediately.
			return &k, nil
		}
		if strings.HasPrefix(k.Fingerprint, normalizedID) {
			matches = append(matches, k)
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no key found matching %q", keyID)
	case 1:
		return &matches[0], nil
	default:
		var fps []string
		for _, m := range matches {
			fps = append(fps, m.Fingerprint)
		}
		return nil, fmt.Errorf("ambiguous key prefix %q matches %d keys: %s",
			keyID, len(matches), strings.Join(fps, ", "))
	}
}
