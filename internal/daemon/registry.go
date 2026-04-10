// Package daemon provides the nix-key daemon's device registry.
//
// The registry maintains an in-memory set of paired devices with their
// cert paths and connection info. Devices are loaded from two sources:
//   - devices.json (runtime-paired devices, persisted by nix-key pair)
//   - Nix-declared devices (from NixOS module config)
//
// These are merged at startup per FR-064 merge rules.
package daemon

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DeviceSource indicates how a device was added to the registry.
type DeviceSource string

const (
	SourceNixDeclared  DeviceSource = "nix-declared"
	SourceRuntimePaired DeviceSource = "runtime-paired"
)

// Device represents a paired phone with its cert paths and connection info.
type Device struct {
	ID              string       `json:"id"`
	Name            string       `json:"name"`
	TailscaleIP     string       `json:"tailscaleIp"`
	ListenPort      int          `json:"listenPort"`
	CertFingerprint string       `json:"certFingerprint"`
	CertPath        string       `json:"certPath"`
	ClientCertPath  string       `json:"clientCertPath"`
	ClientKeyPath   string       `json:"clientKeyPath"`
	LastSeen        *time.Time   `json:"lastSeen,omitempty"`
	Source          DeviceSource `json:"source"`
}

// Registry is a thread-safe in-memory registry of paired devices.
type Registry struct {
	mu      sync.RWMutex
	devices map[string]*Device // keyed by device ID
	byFP    map[string]string  // certFingerprint -> device ID
}

// NewRegistry creates an empty device registry.
func NewRegistry() *Registry {
	return &Registry{
		devices: make(map[string]*Device),
		byFP:    make(map[string]string),
	}
}

// Add adds a device to the registry. If a device with the same ID already
// exists, it is replaced.
func (r *Registry) Add(dev Device) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Remove old fingerprint index if replacing
	if old, ok := r.devices[dev.ID]; ok {
		delete(r.byFP, old.CertFingerprint)
	}

	d := dev // copy
	r.devices[dev.ID] = &d
	r.byFP[dev.CertFingerprint] = dev.ID
}

// Get returns a device by ID.
func (r *Registry) Get(id string) (Device, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	dev, ok := r.devices[id]
	if !ok {
		return Device{}, false
	}
	return *dev, true
}

// Remove removes a device from the registry by ID.
func (r *Registry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if dev, ok := r.devices[id]; ok {
		delete(r.byFP, dev.CertFingerprint)
		delete(r.devices, id)
	}
}

// LookupByKeyFingerprint returns the device with the given cert fingerprint.
func (r *Registry) LookupByKeyFingerprint(certFingerprint string) (Device, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	id, ok := r.byFP[certFingerprint]
	if !ok {
		return Device{}, false
	}
	dev := r.devices[id]
	return *dev, true
}

// ListAll returns all devices in the registry.
func (r *Registry) ListAll() []Device {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Device, 0, len(r.devices))
	for _, dev := range r.devices {
		result = append(result, *dev)
	}
	return result
}

// ListReachable returns all devices that have a TailscaleIP and ListenPort
// configured, meaning they can potentially be dialed.
func (r *Registry) ListReachable() []Device {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Device
	for _, dev := range r.devices {
		if dev.TailscaleIP != "" && dev.ListenPort > 0 {
			result = append(result, *dev)
		}
	}
	return result
}

// UpdateLastSeen sets the LastSeen timestamp for a device.
func (r *Registry) UpdateLastSeen(id string, t time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if dev, ok := r.devices[id]; ok {
		ts := t
		dev.LastSeen = &ts
	}
}

// UpdateIP updates a device's Tailscale IP and sets LastSeen (FR-E12).
// This is called on successful reconnect when the phone's IP has changed.
func (r *Registry) UpdateIP(id string, newIP string, t time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if dev, ok := r.devices[id]; ok {
		dev.TailscaleIP = newIP
		ts := t
		dev.LastSeen = &ts
	}
}

// Merge combines nix-declared devices and runtime-paired devices into the
// registry according to FR-064 merge rules:
//   - If a device ID exists in both sources, nix-declared wins for
//     ClientCertPath/ClientKeyPath (if set); runtime wins for LastSeen
//     and TailscaleIP.
//   - Devices unique to either source are added as-is.
func (r *Registry) Merge(nixDevices, runtimeDevices []Device) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Clear existing state
	r.devices = make(map[string]*Device)
	r.byFP = make(map[string]string)

	// Index runtime devices by ID
	runtimeByID := make(map[string]*Device)
	for i := range runtimeDevices {
		d := runtimeDevices[i]
		runtimeByID[d.ID] = &d
	}

	// Process nix-declared devices
	for i := range nixDevices {
		nix := nixDevices[i]
		merged := nix
		merged.Source = SourceNixDeclared

		if rt, ok := runtimeByID[nix.ID]; ok {
			// Runtime wins for lastSeen and tailscaleIp
			merged.TailscaleIP = rt.TailscaleIP
			merged.LastSeen = rt.LastSeen

			// Nix wins for cert paths only if set (non-empty)
			if nix.ClientCertPath == "" {
				merged.ClientCertPath = rt.ClientCertPath
			}
			if nix.ClientKeyPath == "" {
				merged.ClientKeyPath = rt.ClientKeyPath
			}

			// Keep runtime values for other fields that nix doesn't set
			if merged.CertPath == "" {
				merged.CertPath = rt.CertPath
			}
			if merged.ListenPort == 0 {
				merged.ListenPort = rt.ListenPort
			}

			delete(runtimeByID, nix.ID)
		}

		r.devices[merged.ID] = &merged
		r.byFP[merged.CertFingerprint] = merged.ID
	}

	// Add remaining runtime-only devices
	for _, rt := range runtimeByID {
		d := *rt
		d.Source = SourceRuntimePaired
		r.devices[d.ID] = &d
		r.byFP[d.CertFingerprint] = d.ID
	}
}

// LoadDevicesFromJSON reads devices from a JSON file.
// If the file does not exist, an empty slice is returned (not an error).
func LoadDevicesFromJSON(path string) ([]Device, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var devices []Device
	if err := json.Unmarshal(data, &devices); err != nil {
		return nil, err
	}
	return devices, nil
}

// SaveToJSON writes only runtime-paired devices to a JSON file.
// Nix-declared devices are not persisted (they come from NixOS config).
func (r *Registry) SaveToJSON(path string) error {
	r.mu.RLock()
	var runtimeDevices []Device
	for _, dev := range r.devices {
		if dev.Source == SourceRuntimePaired {
			runtimeDevices = append(runtimeDevices, *dev)
		}
	}
	r.mu.RUnlock()

	if runtimeDevices == nil {
		runtimeDevices = []Device{}
	}

	data, err := json.MarshalIndent(runtimeDevices, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
