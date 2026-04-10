package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRegistryAddAndLookup(t *testing.T) {
	r := NewRegistry()

	dev := Device{
		ID:              "abc123",
		Name:            "Pixel 8",
		TailscaleIP:     "100.64.0.1",
		ListenPort:      29418,
		CertFingerprint: "sha256:abc123",
		CertPath:        "/tmp/cert.pem",
		ClientCertPath:  "/tmp/client.pem",
		ClientKeyPath:   "/tmp/client-key.pem",
		Source:          SourceRuntimePaired,
	}

	r.Add(dev)

	got, ok := r.LookupByKeyFingerprint("sha256:abc123")
	if !ok {
		t.Fatal("expected to find device by cert fingerprint")
	}
	if got.Name != "Pixel 8" {
		t.Errorf("got name %q, want %q", got.Name, "Pixel 8")
	}
	if got.TailscaleIP != "100.64.0.1" {
		t.Errorf("got IP %q, want %q", got.TailscaleIP, "100.64.0.1")
	}
}

func TestRegistryLookupNotFound(t *testing.T) {
	r := NewRegistry()

	_, ok := r.LookupByKeyFingerprint("sha256:nonexistent")
	if ok {
		t.Fatal("expected not found for nonexistent fingerprint")
	}
}

func TestRegistryRemove(t *testing.T) {
	r := NewRegistry()

	dev := Device{
		ID:              "abc123",
		Name:            "Pixel 8",
		CertFingerprint: "sha256:abc123",
		Source:          SourceRuntimePaired,
	}

	r.Add(dev)
	r.Remove("abc123")

	_, ok := r.LookupByKeyFingerprint("sha256:abc123")
	if ok {
		t.Fatal("expected device to be removed")
	}
}

func TestRegistryUpdateLastSeen(t *testing.T) {
	r := NewRegistry()

	dev := Device{
		ID:              "abc123",
		Name:            "Pixel 8",
		CertFingerprint: "sha256:abc123",
		Source:          SourceRuntimePaired,
	}

	r.Add(dev)

	now := time.Now()
	r.UpdateLastSeen("abc123", now)

	got, ok := r.LookupByKeyFingerprint("sha256:abc123")
	if !ok {
		t.Fatal("expected to find device")
	}
	if got.LastSeen == nil {
		t.Fatal("expected LastSeen to be set")
	}
	if !got.LastSeen.Equal(now) {
		t.Errorf("got LastSeen %v, want %v", got.LastSeen, now)
	}
}

func TestRegistryUpdateIP(t *testing.T) {
	r := NewRegistry()

	dev := Device{
		ID:              "abc123",
		Name:            "Pixel 8",
		TailscaleIP:     "100.64.0.1",
		CertFingerprint: "sha256:abc123",
		Source:          SourceRuntimePaired,
	}

	r.Add(dev)

	now := time.Now()
	r.UpdateIP("abc123", "100.64.0.99", now)

	got, _ := r.LookupByKeyFingerprint("sha256:abc123")
	if got.TailscaleIP != "100.64.0.99" {
		t.Errorf("got IP %q, want %q", got.TailscaleIP, "100.64.0.99")
	}
	if got.LastSeen == nil || !got.LastSeen.Equal(now) {
		t.Error("expected LastSeen to be updated on IP change")
	}
}

func TestRegistryListReachable(t *testing.T) {
	r := NewRegistry()

	now := time.Now()
	dev1 := Device{
		ID:              "dev1",
		Name:            "Phone 1",
		TailscaleIP:     "100.64.0.1",
		ListenPort:      29418,
		CertFingerprint: "sha256:dev1",
		Source:          SourceRuntimePaired,
		LastSeen:        &now,
	}
	dev2 := Device{
		ID:              "dev2",
		Name:            "Phone 2",
		TailscaleIP:     "100.64.0.2",
		ListenPort:      29418,
		CertFingerprint: "sha256:dev2",
		Source:          SourceNixDeclared,
	}

	r.Add(dev1)
	r.Add(dev2)

	// ListReachable returns all devices that have connection info
	reachable := r.ListReachable()
	if len(reachable) != 2 {
		t.Errorf("got %d reachable, want 2", len(reachable))
	}
}

func TestRegistryMergeNixAndRuntime(t *testing.T) {
	// Nix-declared device
	nixDevices := []Device{
		{
			ID:              "dev1",
			Name:            "NixPhone",
			TailscaleIP:     "100.64.0.10",
			ListenPort:      29418,
			CertFingerprint: "sha256:dev1",
			ClientCertPath:  "/nix/store/cert.pem",
			ClientKeyPath:   "/nix/store/key.pem",
			Source:          SourceNixDeclared,
		},
	}

	// Runtime device with same ID but different fields
	lastSeen := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)
	runtimeDevices := []Device{
		{
			ID:              "dev1",
			Name:            "RuntimePhone",
			TailscaleIP:     "100.64.0.20",
			ListenPort:      29418,
			CertFingerprint: "sha256:dev1",
			ClientCertPath:  "/home/user/cert.pem",
			ClientKeyPath:   "/home/user/key.pem",
			LastSeen:        &lastSeen,
			Source:          SourceRuntimePaired,
		},
	}

	r := NewRegistry()
	r.Merge(nixDevices, runtimeDevices)

	got, ok := r.LookupByKeyFingerprint("sha256:dev1")
	if !ok {
		t.Fatal("expected to find merged device")
	}

	// Nix-declared wins for clientCertPath/clientKeyPath (when set)
	if got.ClientCertPath != "/nix/store/cert.pem" {
		t.Errorf("clientCertPath: got %q, want nix-declared %q", got.ClientCertPath, "/nix/store/cert.pem")
	}
	if got.ClientKeyPath != "/nix/store/key.pem" {
		t.Errorf("clientKeyPath: got %q, want nix-declared %q", got.ClientKeyPath, "/nix/store/key.pem")
	}

	// Runtime wins for lastSeen and tailscaleIp
	if got.TailscaleIP != "100.64.0.20" {
		t.Errorf("tailscaleIp: got %q, want runtime %q", got.TailscaleIP, "100.64.0.20")
	}
	if got.LastSeen == nil || !got.LastSeen.Equal(lastSeen) {
		t.Errorf("lastSeen: got %v, want runtime %v", got.LastSeen, lastSeen)
	}

	// Source should be nix-declared since it's declared in nix
	if got.Source != SourceNixDeclared {
		t.Errorf("source: got %q, want %q", got.Source, SourceNixDeclared)
	}
}

func TestRegistryMergeNixCertPathsEmptyDoNotOverride(t *testing.T) {
	// Nix-declared device with empty cert paths (set by pairing, not nix)
	nixDevices := []Device{
		{
			ID:              "dev1",
			Name:            "NixPhone",
			TailscaleIP:     "100.64.0.10",
			CertFingerprint: "sha256:dev1",
			ClientCertPath:  "", // not set by nix
			ClientKeyPath:   "", // not set by nix
			Source:          SourceNixDeclared,
		},
	}

	runtimeDevices := []Device{
		{
			ID:              "dev1",
			Name:            "RuntimePhone",
			TailscaleIP:     "100.64.0.20",
			CertFingerprint: "sha256:dev1",
			ClientCertPath:  "/home/user/cert.pem",
			ClientKeyPath:   "/home/user/key.pem",
			Source:          SourceRuntimePaired,
		},
	}

	r := NewRegistry()
	r.Merge(nixDevices, runtimeDevices)

	got, _ := r.LookupByKeyFingerprint("sha256:dev1")

	// When nix cert paths are empty, runtime values are kept
	if got.ClientCertPath != "/home/user/cert.pem" {
		t.Errorf("clientCertPath: got %q, want runtime value", got.ClientCertPath)
	}
	if got.ClientKeyPath != "/home/user/key.pem" {
		t.Errorf("clientKeyPath: got %q, want runtime value", got.ClientKeyPath)
	}
}

func TestRegistryMergeDisjointDevices(t *testing.T) {
	nixDevices := []Device{
		{
			ID:              "nix-only",
			Name:            "NixOnly",
			CertFingerprint: "sha256:nix-only",
			Source:          SourceNixDeclared,
		},
	}

	runtimeDevices := []Device{
		{
			ID:              "runtime-only",
			Name:            "RuntimeOnly",
			CertFingerprint: "sha256:runtime-only",
			Source:          SourceRuntimePaired,
		},
	}

	r := NewRegistry()
	r.Merge(nixDevices, runtimeDevices)

	_, ok1 := r.LookupByKeyFingerprint("sha256:nix-only")
	_, ok2 := r.LookupByKeyFingerprint("sha256:runtime-only")

	if !ok1 {
		t.Error("expected nix-only device to be present")
	}
	if !ok2 {
		t.Error("expected runtime-only device to be present")
	}
}

func TestRegistryLoadFromJSON(t *testing.T) {
	dir := t.TempDir()
	devicesPath := filepath.Join(dir, "devices.json")

	lastSeen := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)
	devices := []Device{
		{
			ID:              "dev1",
			Name:            "TestPhone",
			TailscaleIP:     "100.64.0.1",
			ListenPort:      29418,
			CertFingerprint: "sha256:dev1",
			CertPath:        "/tmp/cert.pem",
			ClientCertPath:  "/tmp/client.pem",
			ClientKeyPath:   "/tmp/client-key.pem",
			LastSeen:        &lastSeen,
			Source:          SourceRuntimePaired,
		},
	}

	data, err := json.MarshalIndent(devices, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(devicesPath, data, 0600); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadDevicesFromJSON(devicesPath)
	if err != nil {
		t.Fatal(err)
	}

	if len(loaded) != 1 {
		t.Fatalf("got %d devices, want 1", len(loaded))
	}
	if loaded[0].Name != "TestPhone" {
		t.Errorf("got name %q, want %q", loaded[0].Name, "TestPhone")
	}
	if loaded[0].LastSeen == nil || !loaded[0].LastSeen.Equal(lastSeen) {
		t.Errorf("lastSeen mismatch")
	}
}

func TestRegistryLoadFromJSONMissingFile(t *testing.T) {
	loaded, err := LoadDevicesFromJSON("/nonexistent/devices.json")
	if err != nil {
		t.Fatalf("missing file should return empty list, got error: %v", err)
	}
	if len(loaded) != 0 {
		t.Errorf("got %d devices, want 0", len(loaded))
	}
}

func TestRegistrySaveToJSON(t *testing.T) {
	dir := t.TempDir()
	devicesPath := filepath.Join(dir, "devices.json")

	r := NewRegistry()
	r.Add(Device{
		ID:              "dev1",
		Name:            "Phone1",
		TailscaleIP:     "100.64.0.1",
		CertFingerprint: "sha256:dev1",
		Source:          SourceRuntimePaired,
	})
	r.Add(Device{
		ID:              "dev2",
		Name:            "NixPhone",
		TailscaleIP:     "100.64.0.2",
		CertFingerprint: "sha256:dev2",
		Source:          SourceNixDeclared,
	})

	if err := r.SaveToJSON(devicesPath); err != nil {
		t.Fatal(err)
	}

	// Only runtime-paired devices should be saved
	loaded, err := LoadDevicesFromJSON(devicesPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 {
		t.Fatalf("got %d saved devices, want 1 (only runtime-paired)", len(loaded))
	}
	if loaded[0].Name != "Phone1" {
		t.Errorf("got name %q, want %q", loaded[0].Name, "Phone1")
	}
}

func TestRegistryIPUpdateOnReconnect(t *testing.T) {
	// Simulates FR-E12: IP update on successful reconnect
	r := NewRegistry()

	dev := Device{
		ID:              "dev1",
		Name:            "MobilePhone",
		TailscaleIP:     "100.64.0.1",
		ListenPort:      29418,
		CertFingerprint: "sha256:dev1",
		Source:          SourceRuntimePaired,
	}
	r.Add(dev)

	// Simulate reconnect with new IP
	reconnectTime := time.Now()
	r.UpdateIP("dev1", "100.64.0.50", reconnectTime)

	got, _ := r.LookupByKeyFingerprint("sha256:dev1")
	if got.TailscaleIP != "100.64.0.50" {
		t.Errorf("IP after reconnect: got %q, want %q", got.TailscaleIP, "100.64.0.50")
	}
	if got.LastSeen == nil || !got.LastSeen.Equal(reconnectTime) {
		t.Error("LastSeen should be updated on IP change")
	}

	// Save and reload to verify IP persists
	dir := t.TempDir()
	devicesPath := filepath.Join(dir, "devices.json")
	if err := r.SaveToJSON(devicesPath); err != nil {
		t.Fatal(err)
	}

	loaded, _ := LoadDevicesFromJSON(devicesPath)
	if len(loaded) != 1 {
		t.Fatalf("got %d devices, want 1", len(loaded))
	}
	if loaded[0].TailscaleIP != "100.64.0.50" {
		t.Errorf("persisted IP: got %q, want %q", loaded[0].TailscaleIP, "100.64.0.50")
	}
}

func TestRegistryUpdateNonexistentDevice(t *testing.T) {
	r := NewRegistry()

	// These should not panic
	r.UpdateLastSeen("nonexistent", time.Now())
	r.UpdateIP("nonexistent", "100.64.0.1", time.Now())
	r.Remove("nonexistent")
}

func TestRegistryConcurrentAccess(t *testing.T) {
	r := NewRegistry()

	dev := Device{
		ID:              "dev1",
		Name:            "Phone",
		CertFingerprint: "sha256:dev1",
		TailscaleIP:     "100.64.0.1",
		Source:          SourceRuntimePaired,
	}
	r.Add(dev)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			r.LookupByKeyFingerprint("sha256:dev1")
			r.ListReachable()
		}
		close(done)
	}()

	for i := 0; i < 100; i++ {
		r.UpdateLastSeen("dev1", time.Now())
		r.UpdateIP("dev1", "100.64.0.1", time.Now())
	}

	<-done
}
