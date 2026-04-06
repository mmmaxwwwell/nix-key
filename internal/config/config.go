// Package config loads, validates, and provides nix-key configuration.
//
// Configuration is resolved in three layers (later wins):
//  1. Hardcoded defaults
//  2. Config file (~/.config/nix-key/config.json)
//  3. Environment variables (NIXKEY_ prefix)
//
// The daemon fails fast on startup if the config is invalid, reporting
// all invalid fields at once.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	nixerrors "github.com/phaedrus-raznikov/nix-key/internal/errors"
)

// Config holds all nix-key daemon configuration.
type Config struct {
	Port                int     `json:"port"`
	TailscaleInterface  string  `json:"tailscaleInterface"`
	AllowKeyListing     bool    `json:"allowKeyListing"`
	SignTimeout         int     `json:"signTimeout"`
	ConnectionTimeout   int     `json:"connectionTimeout"`
	SocketPath          string  `json:"socketPath"`
	ControlSocketPath   string  `json:"controlSocketPath"`
	LogLevel            string  `json:"logLevel"`
	OtelEndpoint        *string `json:"otelEndpoint"`
	JaegerEnable        bool    `json:"jaegerEnable"`
	AgeKeyFile          string  `json:"ageKeyFile"`
	TailscaleAuthKeyFile *string `json:"tailscaleAuthKeyFile"`
	CertExpiry          string  `json:"certExpiry"`
}

// defaults returns a Config populated with hardcoded default values.
func defaults() Config {
	return Config{
		Port:               29418,
		TailscaleInterface: "tailscale0",
		AllowKeyListing:    true,
		SignTimeout:        30,
		ConnectionTimeout:  10,
		LogLevel:           "info",
		AgeKeyFile:         "~/.local/state/nix-key/age-identity.txt",
		CertExpiry:         "365d",
	}
}

// Load reads configuration from the given file path, applies defaults first,
// then overlays file values, then environment variable overrides, and
// validates the result. Returns a ConfigError listing all invalid fields.
func Load(path string) (*Config, error) {
	cfg := defaults()

	// Layer 2: config file.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nixerrors.NewConfigError("ERR_CONFIG_READ",
			fmt.Sprintf("failed to read config file %s: %s", path, err.Error()))
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, nixerrors.NewConfigError("ERR_CONFIG_PARSE",
			fmt.Sprintf("failed to parse config file %s: %s", path, err.Error()))
	}

	// Layer 3: environment variable overrides.
	applyEnvOverrides(&cfg)

	// Validate.
	if errs := validate(&cfg); len(errs) > 0 {
		msg := "invalid configuration:\n"
		for _, e := range errs {
			msg += "  - " + e + "\n"
		}
		return nil, nixerrors.NewConfigError("ERR_CONFIG_INVALID", strings.TrimRight(msg, "\n"))
	}

	return &cfg, nil
}

// envMapping maps NIXKEY_ env var names to config field appliers.
var envMapping = []struct {
	envKey string
	apply  func(val string, cfg *Config) error
}{
	{"NIXKEY_PORT", func(v string, c *Config) error {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("NIXKEY_PORT: %w", err)
		}
		c.Port = n
		return nil
	}},
	{"NIXKEY_TAILSCALE_INTERFACE", func(v string, c *Config) error {
		c.TailscaleInterface = v
		return nil
	}},
	{"NIXKEY_ALLOW_KEY_LISTING", func(v string, c *Config) error {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("NIXKEY_ALLOW_KEY_LISTING: %w", err)
		}
		c.AllowKeyListing = b
		return nil
	}},
	{"NIXKEY_SIGN_TIMEOUT", func(v string, c *Config) error {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("NIXKEY_SIGN_TIMEOUT: %w", err)
		}
		c.SignTimeout = n
		return nil
	}},
	{"NIXKEY_CONNECTION_TIMEOUT", func(v string, c *Config) error {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("NIXKEY_CONNECTION_TIMEOUT: %w", err)
		}
		c.ConnectionTimeout = n
		return nil
	}},
	{"NIXKEY_SOCKET_PATH", func(v string, c *Config) error {
		c.SocketPath = v
		return nil
	}},
	{"NIXKEY_CONTROL_SOCKET_PATH", func(v string, c *Config) error {
		c.ControlSocketPath = v
		return nil
	}},
	{"NIXKEY_LOG_LEVEL", func(v string, c *Config) error {
		c.LogLevel = v
		return nil
	}},
	{"NIXKEY_OTEL_ENDPOINT", func(v string, c *Config) error {
		c.OtelEndpoint = &v
		return nil
	}},
	{"NIXKEY_JAEGER_ENABLE", func(v string, c *Config) error {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("NIXKEY_JAEGER_ENABLE: %w", err)
		}
		c.JaegerEnable = b
		return nil
	}},
	{"NIXKEY_AGE_KEY_FILE", func(v string, c *Config) error {
		c.AgeKeyFile = v
		return nil
	}},
	{"NIXKEY_TAILSCALE_AUTH_KEY_FILE", func(v string, c *Config) error {
		c.TailscaleAuthKeyFile = &v
		return nil
	}},
	{"NIXKEY_CERT_EXPIRY", func(v string, c *Config) error {
		c.CertExpiry = v
		return nil
	}},
}

func applyEnvOverrides(cfg *Config) {
	for _, m := range envMapping {
		if v, ok := os.LookupEnv(m.envKey); ok {
			// Ignore parse errors here; validation will catch invalid values.
			_ = m.apply(v, cfg)
		}
	}
}

var validLogLevels = map[string]bool{
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
	"fatal": true,
}

func validate(cfg *Config) []string {
	var errs []string

	if cfg.Port < 1 || cfg.Port > 65535 {
		errs = append(errs, fmt.Sprintf("port: must be 1-65535, got %d", cfg.Port))
	}
	if cfg.TailscaleInterface == "" {
		errs = append(errs, "tailscaleInterface: required, must not be empty")
	}
	if cfg.SignTimeout < 1 {
		errs = append(errs, fmt.Sprintf("signTimeout: must be >= 1, got %d", cfg.SignTimeout))
	}
	if cfg.ConnectionTimeout < 1 {
		errs = append(errs, fmt.Sprintf("connectionTimeout: must be >= 1, got %d", cfg.ConnectionTimeout))
	}
	if cfg.SocketPath == "" {
		errs = append(errs, "socketPath: required, must not be empty")
	}
	if cfg.ControlSocketPath == "" {
		errs = append(errs, "controlSocketPath: required, must not be empty")
	}
	if !validLogLevels[strings.ToLower(cfg.LogLevel)] {
		errs = append(errs, fmt.Sprintf("logLevel: must be one of debug, info, warn, error, fatal; got %q", cfg.LogLevel))
	}
	if cfg.CertExpiry == "" {
		errs = append(errs, "certExpiry: required, must not be empty")
	}

	return errs
}

// RedactedFields returns a map of sensitive config field names to
// "present" or "missing" strings, suitable for logging.
func (c *Config) RedactedFields() map[string]string {
	redacted := make(map[string]string)

	redacted["ageKeyFile"] = presentOrMissing(c.AgeKeyFile)

	tailscaleAuthKey := ""
	if c.TailscaleAuthKeyFile != nil {
		tailscaleAuthKey = *c.TailscaleAuthKeyFile
	}
	redacted["tailscaleAuthKeyFile"] = presentOrMissing(tailscaleAuthKey)

	return redacted
}

func presentOrMissing(val string) string {
	if val != "" {
		return "present"
	}
	return "missing"
}
