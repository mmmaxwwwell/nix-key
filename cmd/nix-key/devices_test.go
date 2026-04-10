package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/phaedrus-raznikov/nix-key/internal/daemon"
)

func TestFormatDevicesTable(t *testing.T) {
	now := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	old := time.Date(2025, 6, 14, 8, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		devices []daemon.DeviceInfo
		want    []string // substrings that must appear in output
		notWant []string // substrings that must NOT appear
	}{
		{
			name:    "empty list",
			devices: []daemon.DeviceInfo{},
			want:    []string{"No devices paired"},
		},
		{
			name: "single device with all fields",
			devices: []daemon.DeviceInfo{
				{
					ID:              "abc123",
					Name:            "pixel-7",
					TailscaleIP:     "100.64.0.1",
					CertFingerprint: "SHA256:abc123def456abc123def456abc123def456abc123def456abc123def456abcd",
					LastSeen:        &now,
					Source:          "runtime-paired",
				},
			},
			want: []string{
				"pixel-7",
				"100.64.0.1",
				"SHA256:abc123de",
				"2025-06-15",
				"runtime-paired",
				"NAME",
				"TAILSCALE IP",
				"CERT FINGERPRINT",
				"LAST SEEN",
				"STATUS",
				"SOURCE",
			},
		},
		{
			name: "multiple devices",
			devices: []daemon.DeviceInfo{
				{
					ID:              "abc123",
					Name:            "pixel-7",
					TailscaleIP:     "100.64.0.1",
					CertFingerprint: "SHA256:aabbccdd",
					LastSeen:        &now,
					Source:          "runtime-paired",
				},
				{
					ID:              "def456",
					Name:            "samsung-s24",
					TailscaleIP:     "100.64.0.2",
					CertFingerprint: "SHA256:11223344",
					LastSeen:        &old,
					Source:          "nix-declared",
				},
			},
			want: []string{"pixel-7", "samsung-s24", "100.64.0.1", "100.64.0.2"},
		},
		{
			name: "device with no lastSeen",
			devices: []daemon.DeviceInfo{
				{
					ID:              "abc123",
					Name:            "pixel-7",
					TailscaleIP:     "100.64.0.1",
					CertFingerprint: "SHA256:aabbccdd",
					LastSeen:        nil,
					Source:          "runtime-paired",
				},
			},
			want: []string{"pixel-7", "never"},
		},
		{
			name: "device with empty tailscale IP",
			devices: []daemon.DeviceInfo{
				{
					ID:              "abc123",
					Name:            "pixel-7",
					TailscaleIP:     "",
					CertFingerprint: "SHA256:aabbccdd",
					LastSeen:        nil,
					Source:          "nix-declared",
				},
			},
			want: []string{"pixel-7", "-"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf strings.Builder
			formatDevicesTable(&buf, tt.devices, nil)
			output := buf.String()

			for _, s := range tt.want {
				if !strings.Contains(output, s) {
					t.Errorf("output should contain %q, got:\n%s", s, output)
				}
			}
			for _, s := range tt.notWant {
				if strings.Contains(output, s) {
					t.Errorf("output should NOT contain %q, got:\n%s", s, output)
				}
			}
		})
	}
}

func TestParseDeviceInfoFromResponse(t *testing.T) {
	now := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)

	devices := []daemon.DeviceInfo{
		{
			ID:              "abc123",
			Name:            "pixel-7",
			TailscaleIP:     "100.64.0.1",
			CertFingerprint: "SHA256:aabbccdd",
			LastSeen:        &now,
			Source:          "runtime-paired",
		},
	}

	// Simulate what ControlClient returns: Data is json.RawMessage after re-marshal
	dataBytes, err := json.Marshal(devices)
	if err != nil {
		t.Fatal(err)
	}
	var rawData interface{}
	if err := json.Unmarshal(dataBytes, &rawData); err != nil {
		t.Fatal(err)
	}

	resp := &daemon.Response{
		Status: "ok",
		Data:   rawData,
	}

	got, err := parseDeviceInfos(resp)
	if err != nil {
		t.Fatalf("parseDeviceInfos: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 device, got %d", len(got))
	}
	if got[0].Name != "pixel-7" {
		t.Errorf("expected name pixel-7, got %s", got[0].Name)
	}
	if got[0].TailscaleIP != "100.64.0.1" {
		t.Errorf("expected IP 100.64.0.1, got %s", got[0].TailscaleIP)
	}
}

func TestParseDeviceInfoFromErrorResponse(t *testing.T) {
	resp := &daemon.Response{
		Status: "error",
		Error:  "something went wrong",
	}

	_, err := parseDeviceInfos(resp)
	if err == nil {
		t.Fatal("expected error for error response")
	}
	if !strings.Contains(err.Error(), "something went wrong") {
		t.Errorf("error should contain daemon message, got: %v", err)
	}
}
