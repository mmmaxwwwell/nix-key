package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	nixerrors "github.com/phaedrus-raznikov/nix-key/internal/errors"
)

func writeConfigFile(t *testing.T, dir string, cfg map[string]interface{}) string {
	t.Helper()
	configDir := filepath.Join(dir, ".config", "nix-key")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(configDir, "config.json")
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestValidConfigLoads(t *testing.T) {
	dir := t.TempDir()

	writeConfigFile(t, dir, map[string]interface{}{
		"port":               29418,
		"tailscaleInterface": "tailscale0",
		"allowKeyListing":    true,
		"signTimeout":        30,
		"connectionTimeout":  10,
		"socketPath":         "/run/user/1000/nix-key/agent.sock",
		"controlSocketPath":  "/run/user/1000/nix-key/control.sock",
		"logLevel":           "info",
		"ageKeyFile":         "/home/user/.local/state/nix-key/age-identity.txt",
		"certExpiry":         "365d",
	})

	cfg, err := Load(filepath.Join(dir, ".config", "nix-key", "config.json"))
	if err != nil {
		t.Fatalf("expected valid config to load, got error: %v", err)
	}

	if cfg.Port != 29418 {
		t.Errorf("port = %d, want 29418", cfg.Port)
	}
	if cfg.TailscaleInterface != "tailscale0" {
		t.Errorf("tailscaleInterface = %q, want %q", cfg.TailscaleInterface, "tailscale0")
	}
	if cfg.AllowKeyListing != true {
		t.Error("allowKeyListing should be true")
	}
	if cfg.SignTimeout != 30 {
		t.Errorf("signTimeout = %d, want 30", cfg.SignTimeout)
	}
	if cfg.ConnectionTimeout != 10 {
		t.Errorf("connectionTimeout = %d, want 10", cfg.ConnectionTimeout)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("logLevel = %q, want %q", cfg.LogLevel, "info")
	}
}

func TestDefaultsApplied(t *testing.T) {
	dir := t.TempDir()

	// Write minimal config — defaults should fill in the rest.
	writeConfigFile(t, dir, map[string]interface{}{
		"socketPath":        "/run/user/1000/nix-key/agent.sock",
		"controlSocketPath": "/run/user/1000/nix-key/control.sock",
	})

	cfg, err := Load(filepath.Join(dir, ".config", "nix-key", "config.json"))
	if err != nil {
		t.Fatalf("expected config with defaults to load, got error: %v", err)
	}

	if cfg.Port != 29418 {
		t.Errorf("default port = %d, want 29418", cfg.Port)
	}
	if cfg.TailscaleInterface != "tailscale0" {
		t.Errorf("default tailscaleInterface = %q, want %q", cfg.TailscaleInterface, "tailscale0")
	}
	if cfg.AllowKeyListing != true {
		t.Error("default allowKeyListing should be true")
	}
	if cfg.SignTimeout != 30 {
		t.Errorf("default signTimeout = %d, want 30", cfg.SignTimeout)
	}
	if cfg.ConnectionTimeout != 10 {
		t.Errorf("default connectionTimeout = %d, want 10", cfg.ConnectionTimeout)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("default logLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.CertExpiry != "365d" {
		t.Errorf("default certExpiry = %q, want %q", cfg.CertExpiry, "365d")
	}
	if cfg.AgeKeyFile != "~/.local/state/nix-key/age-identity.txt" {
		t.Errorf("default ageKeyFile = %q, want %q", cfg.AgeKeyFile, "~/.local/state/nix-key/age-identity.txt")
	}
}

func TestMissingRequiredFieldFails(t *testing.T) {
	dir := t.TempDir()

	// Missing socketPath and controlSocketPath (required, no defaults).
	writeConfigFile(t, dir, map[string]interface{}{
		"port": 29418,
	})

	_, err := Load(filepath.Join(dir, ".config", "nix-key", "config.json"))
	if err == nil {
		t.Fatal("expected error for missing required fields")
	}

	// Should be a ConfigError.
	if !nixerrors.IsConfigError(err) {
		t.Errorf("expected ConfigError, got %T: %v", err, err)
	}

	// Error message should mention both missing fields.
	errMsg := err.Error()
	if !containsSubstring(errMsg, "socketPath") {
		t.Errorf("error should mention socketPath: %v", err)
	}
	if !containsSubstring(errMsg, "controlSocketPath") {
		t.Errorf("error should mention controlSocketPath: %v", err)
	}
}

func TestEnvVarOverridesFile(t *testing.T) {
	dir := t.TempDir()

	writeConfigFile(t, dir, map[string]interface{}{
		"port":               29418,
		"socketPath":         "/run/user/1000/nix-key/agent.sock",
		"controlSocketPath":  "/run/user/1000/nix-key/control.sock",
		"logLevel":           "info",
		"tailscaleInterface": "tailscale0",
	})

	t.Setenv("NIXKEY_PORT", "12345")
	t.Setenv("NIXKEY_LOG_LEVEL", "debug")
	t.Setenv("NIXKEY_ALLOW_KEY_LISTING", "false")
	t.Setenv("NIXKEY_SIGN_TIMEOUT", "60")

	cfg, err := Load(filepath.Join(dir, ".config", "nix-key", "config.json"))
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if cfg.Port != 12345 {
		t.Errorf("port = %d, want 12345 (from env)", cfg.Port)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("logLevel = %q, want %q (from env)", cfg.LogLevel, "debug")
	}
	if cfg.AllowKeyListing != false {
		t.Error("allowKeyListing should be false (from env)")
	}
	if cfg.SignTimeout != 60 {
		t.Errorf("signTimeout = %d, want 60 (from env)", cfg.SignTimeout)
	}
}

func TestInvalidPortFails(t *testing.T) {
	dir := t.TempDir()

	writeConfigFile(t, dir, map[string]interface{}{
		"port":               99999,
		"socketPath":         "/run/user/1000/nix-key/agent.sock",
		"controlSocketPath":  "/run/user/1000/nix-key/control.sock",
		"tailscaleInterface": "tailscale0",
	})

	_, err := Load(filepath.Join(dir, ".config", "nix-key", "config.json"))
	if err == nil {
		t.Fatal("expected error for invalid port")
	}
	if !nixerrors.IsConfigError(err) {
		t.Errorf("expected ConfigError, got %T: %v", err, err)
	}
	if !containsSubstring(err.Error(), "port") {
		t.Errorf("error should mention port: %v", err)
	}
}

func TestInvalidLogLevelFails(t *testing.T) {
	dir := t.TempDir()

	writeConfigFile(t, dir, map[string]interface{}{
		"socketPath":         "/run/user/1000/nix-key/agent.sock",
		"controlSocketPath":  "/run/user/1000/nix-key/control.sock",
		"tailscaleInterface": "tailscale0",
		"logLevel":           "verbose",
	})

	_, err := Load(filepath.Join(dir, ".config", "nix-key", "config.json"))
	if err == nil {
		t.Fatal("expected error for invalid logLevel")
	}
	if !nixerrors.IsConfigError(err) {
		t.Errorf("expected ConfigError, got %T: %v", err, err)
	}
}

func TestInvalidSignTimeoutFails(t *testing.T) {
	dir := t.TempDir()

	writeConfigFile(t, dir, map[string]interface{}{
		"socketPath":         "/run/user/1000/nix-key/agent.sock",
		"controlSocketPath":  "/run/user/1000/nix-key/control.sock",
		"tailscaleInterface": "tailscale0",
		"signTimeout":        0,
	})

	_, err := Load(filepath.Join(dir, ".config", "nix-key", "config.json"))
	if err == nil {
		t.Fatal("expected error for invalid signTimeout")
	}
	if !nixerrors.IsConfigError(err) {
		t.Errorf("expected ConfigError, got %T: %v", err, err)
	}
}

func TestMissingConfigFileFails(t *testing.T) {
	_, err := Load("/nonexistent/path/config.json")
	if err == nil {
		t.Fatal("expected error for missing config file")
	}
	if !nixerrors.IsConfigError(err) {
		t.Errorf("expected ConfigError, got %T: %v", err, err)
	}
}

func TestMalformedJSONFails(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".config", "nix-key")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(configDir, "config.json")
	if err := os.WriteFile(path, []byte("{invalid json"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !nixerrors.IsConfigError(err) {
		t.Errorf("expected ConfigError, got %T: %v", err, err)
	}
}

func TestSensitiveFieldsRedacted(t *testing.T) {
	cfg := &Config{
		Port:               29418,
		TailscaleInterface: "tailscale0",
		AllowKeyListing:    true,
		SignTimeout:        30,
		ConnectionTimeout:  10,
		SocketPath:         "/run/user/1000/nix-key/agent.sock",
		ControlSocketPath:  "/run/user/1000/nix-key/control.sock",
		LogLevel:           "info",
		AgeKeyFile:         "/home/user/.local/state/nix-key/age-identity.txt",
		CertExpiry:         "365d",
	}

	redacted := cfg.RedactedFields()
	if redacted["ageKeyFile"] != "present" {
		t.Errorf("ageKeyFile should be redacted as 'present', got %q", redacted["ageKeyFile"])
	}

	cfg.AgeKeyFile = ""
	redacted = cfg.RedactedFields()
	if redacted["ageKeyFile"] != "missing" {
		t.Errorf("empty ageKeyFile should be redacted as 'missing', got %q", redacted["ageKeyFile"])
	}
}

func TestEnvVarStringOverride(t *testing.T) {
	dir := t.TempDir()

	writeConfigFile(t, dir, map[string]interface{}{
		"socketPath":         "/run/user/1000/nix-key/agent.sock",
		"controlSocketPath":  "/run/user/1000/nix-key/control.sock",
		"tailscaleInterface": "tailscale0",
	})

	t.Setenv("NIXKEY_SOCKET_PATH", "/tmp/override.sock")
	t.Setenv("NIXKEY_AGE_KEY_FILE", "/tmp/age.txt")

	cfg, err := Load(filepath.Join(dir, ".config", "nix-key", "config.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.SocketPath != "/tmp/override.sock" {
		t.Errorf("socketPath = %q, want /tmp/override.sock", cfg.SocketPath)
	}
	if cfg.AgeKeyFile != "/tmp/age.txt" {
		t.Errorf("ageKeyFile = %q, want /tmp/age.txt", cfg.AgeKeyFile)
	}
}

func TestMultipleValidationErrors(t *testing.T) {
	dir := t.TempDir()

	// Invalid port AND invalid signTimeout AND missing required fields.
	writeConfigFile(t, dir, map[string]interface{}{
		"port":        99999,
		"signTimeout": -1,
	})

	_, err := Load(filepath.Join(dir, ".config", "nix-key", "config.json"))
	if err == nil {
		t.Fatal("expected error for multiple invalid fields")
	}

	errMsg := err.Error()
	// Should report all errors, not just the first.
	if !containsSubstring(errMsg, "port") {
		t.Errorf("error should mention port: %v", err)
	}
	if !containsSubstring(errMsg, "socketPath") {
		t.Errorf("error should mention socketPath: %v", err)
	}
}

func TestStructTagValidation(t *testing.T) {
	tests := []struct {
		name      string
		cfg       map[string]interface{}
		wantInErr string
	}{
		{
			name: "port 0 fails min tag",
			cfg: map[string]interface{}{
				"port":               0,
				"socketPath":         "/tmp/agent.sock",
				"controlSocketPath":  "/tmp/control.sock",
				"tailscaleInterface": "tailscale0",
			},
			wantInErr: "min",
		},
		{
			name: "port 70000 fails max tag",
			cfg: map[string]interface{}{
				"port":               70000,
				"socketPath":         "/tmp/agent.sock",
				"controlSocketPath":  "/tmp/control.sock",
				"tailscaleInterface": "tailscale0",
			},
			wantInErr: "max",
		},
		{
			name: "empty tailscaleInterface fails required tag",
			cfg: map[string]interface{}{
				"socketPath":         "/tmp/agent.sock",
				"controlSocketPath":  "/tmp/control.sock",
				"tailscaleInterface": "",
			},
			wantInErr: "required",
		},
		{
			name: "logLevel trace fails oneof tag",
			cfg: map[string]interface{}{
				"socketPath":         "/tmp/agent.sock",
				"controlSocketPath":  "/tmp/control.sock",
				"tailscaleInterface": "tailscale0",
				"logLevel":           "trace",
			},
			wantInErr: "oneof",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeConfigFile(t, dir, tc.cfg)

			_, err := Load(filepath.Join(dir, ".config", "nix-key", "config.json"))
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !containsSubstring(err.Error(), tc.wantInErr) {
				t.Errorf("error %q should contain %q", err.Error(), tc.wantInErr)
			}
		})
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
