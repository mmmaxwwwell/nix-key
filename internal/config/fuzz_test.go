package config

import (
	"encoding/json"
	"testing"
)

// FuzzConfigParse tests that arbitrary JSON doesn't panic when parsed as
// a nix-key Config struct.
func FuzzConfigParse(f *testing.F) {
	f.Add([]byte(`{"port":29418,"tailscaleInterface":"tailscale0","allowKeyListing":true,"signTimeout":30,"connectionTimeout":10,"socketPath":"/tmp/agent.sock","controlSocketPath":"/tmp/ctl.sock","logLevel":"info","ageKeyFile":"~/.local/state/nix-key/age-identity.txt","certExpiry":"365d"}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`null`))
	f.Add([]byte(``))
	f.Add([]byte(`{"port":"not-a-number"}`))
	f.Add([]byte(`{"logLevel":"invalid","signTimeout":-1}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var cfg Config
		_ = json.Unmarshal(data, &cfg)
	})
}
