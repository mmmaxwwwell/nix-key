package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// sensitiveFields lists config field JSON keys whose values should be masked.
var sensitiveFields = map[string]bool{
	"ageKeyFile":           true,
	"tailscaleAuthKeyFile": true,
}

// defaultConfigPath returns the default config file path based on
// XDG_CONFIG_HOME (typically ~/.config).
func defaultConfigPath() string {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, ".config")
	}
	return filepath.Join(configDir, "nix-key", "config.json")
}

// runConfig reads the config file, pretty-prints it with sensitive paths masked.
func runConfig(configPath string, w io.Writer) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse into ordered map to preserve JSON field order.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	// Print each field with masking for sensitive values.
	for key, val := range raw {
		if sensitiveFields[key] {
			// Unmarshal to check if string is empty/null.
			var s *string
			if err := json.Unmarshal(val, &s); err == nil {
				if s == nil || *s == "" {
					fmt.Fprintf(w, "%-22s %s\n", key+":", "missing")
				} else {
					fmt.Fprintf(w, "%-22s %s\n", key+":", "present")
				}
			} else {
				fmt.Fprintf(w, "%-22s %s\n", key+":", "present")
			}
			continue
		}

		// For nullable optional fields, check for null.
		var s *string
		if err := json.Unmarshal(val, &s); err == nil && s == nil {
			fmt.Fprintf(w, "%-22s %s\n", key+":", "not set")
			continue
		}

		// Print the value as-is (strip quotes from strings).
		var strVal string
		if err := json.Unmarshal(val, &strVal); err == nil {
			fmt.Fprintf(w, "%-22s %s\n", key+":", strVal)
			continue
		}

		// For non-string values (bool, number), print raw JSON.
		fmt.Fprintf(w, "%-22s %s\n", key+":", string(val))
	}

	return nil
}
