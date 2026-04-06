package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunConfigBasic(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	content := `{
  "port": 29418,
  "tailscaleInterface": "tailscale0",
  "allowKeyListing": true,
  "signTimeout": 30,
  "connectionTimeout": 10,
  "socketPath": "/run/user/1000/nix-key/agent.sock",
  "controlSocketPath": "/run/user/1000/nix-key/control.sock",
  "logLevel": "info",
  "ageKeyFile": "/home/user/.local/state/nix-key/age-identity.txt",
  "certExpiry": "365d"
}`
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	var buf strings.Builder
	err := runConfig(configPath, &buf)
	if err != nil {
		t.Fatalf("runConfig: %v", err)
	}

	output := buf.String()

	// Non-sensitive fields should appear with their values.
	for _, want := range []string{
		"port", "29418",
		"tailscaleInterface", "tailscale0",
		"allowKeyListing", "true",
		"signTimeout", "30",
		"connectionTimeout", "10",
		"socketPath",
		"controlSocketPath",
		"logLevel", "info",
		"certExpiry", "365d",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output should contain %q, got:\n%s", want, output)
		}
	}

	// Sensitive path should be masked.
	if strings.Contains(output, "/home/user/.local/state/nix-key/age-identity.txt") {
		t.Errorf("output should mask ageKeyFile path, got:\n%s", output)
	}
	if !strings.Contains(output, "present") {
		t.Errorf("output should show 'present' for sensitive fields, got:\n%s", output)
	}
}

func TestRunConfigWithOtelAndTailscaleAuth(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	content := `{
  "port": 29418,
  "tailscaleInterface": "tailscale0",
  "allowKeyListing": false,
  "signTimeout": 30,
  "connectionTimeout": 10,
  "socketPath": "/run/user/1000/nix-key/agent.sock",
  "controlSocketPath": "/run/user/1000/nix-key/control.sock",
  "logLevel": "debug",
  "otelEndpoint": "http://localhost:4317",
  "ageKeyFile": "/home/user/.local/state/nix-key/age-identity.txt",
  "tailscaleAuthKeyFile": "/etc/nix-key/ts-auth-key",
  "certExpiry": "365d"
}`
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	var buf strings.Builder
	err := runConfig(configPath, &buf)
	if err != nil {
		t.Fatalf("runConfig: %v", err)
	}

	output := buf.String()

	// otelEndpoint should show the value (it's not a sensitive path).
	if !strings.Contains(output, "http://localhost:4317") {
		t.Errorf("output should show otelEndpoint value, got:\n%s", output)
	}

	// tailscaleAuthKeyFile should be masked.
	if strings.Contains(output, "/etc/nix-key/ts-auth-key") {
		t.Errorf("output should mask tailscaleAuthKeyFile path, got:\n%s", output)
	}

	if !strings.Contains(output, "allowKeyListing") {
		t.Errorf("output should contain allowKeyListing, got:\n%s", output)
	}
}

func TestRunConfigMissingFile(t *testing.T) {
	var buf strings.Builder
	err := runConfig("/tmp/nonexistent-config-12345.json", &buf)
	if err == nil {
		t.Fatal("expected error for missing config file")
	}
}

func TestRunConfigNullOptionalFields(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	// Config with explicit null for optional fields.
	content := `{
  "port": 29418,
  "tailscaleInterface": "tailscale0",
  "allowKeyListing": true,
  "signTimeout": 30,
  "connectionTimeout": 10,
  "socketPath": "/run/user/1000/nix-key/agent.sock",
  "controlSocketPath": "/run/user/1000/nix-key/control.sock",
  "logLevel": "info",
  "otelEndpoint": null,
  "ageKeyFile": "",
  "certExpiry": "365d"
}`
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	var buf strings.Builder
	err := runConfig(configPath, &buf)
	if err != nil {
		t.Fatalf("runConfig: %v", err)
	}

	output := buf.String()

	// otelEndpoint should show "not set" or similar when null.
	if !strings.Contains(output, "not set") {
		t.Errorf("output should show 'not set' for null optional fields, got:\n%s", output)
	}

	// ageKeyFile empty should show "missing".
	if !strings.Contains(output, "missing") {
		t.Errorf("output should show 'missing' for empty sensitive fields, got:\n%s", output)
	}
}
