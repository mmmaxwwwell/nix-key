package daemon

import (
	"bufio"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Request represents a control socket command from the CLI.
type Request struct {
	Command  string `json:"command"`
	DeviceID string `json:"deviceId,omitempty"`
}

// Response represents a control socket response from the daemon.
type Response struct {
	Status string      `json:"status"`
	Error  string      `json:"error,omitempty"`
	Data   interface{} `json:"data,omitempty"`
}

// DeviceInfo is the wire format for a device in list-devices responses.
type DeviceInfo struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	TailscaleIP     string     `json:"tailscaleIp"`
	ListenPort      int        `json:"listenPort"`
	CertFingerprint string     `json:"certFingerprint"`
	LastSeen        *time.Time `json:"lastSeen,omitempty"`
	Source          string     `json:"source"`
}

// KeyInfo is the wire format for a cached SSH key in get-keys responses.
type KeyInfo struct {
	Fingerprint string `json:"fingerprint"`
	KeyType     string `json:"keyType"`
	DisplayName string `json:"displayName"`
	DeviceID    string `json:"deviceId"`
	PublicKey   string `json:"publicKey,omitempty"` // SSH authorized_keys format
}

// CertWarning represents a certificate expiry warning for a device.
type CertWarning struct {
	DeviceID   string    `json:"deviceId"`
	DeviceName string    `json:"deviceName"`
	CertType   string    `json:"certType"` // "server" or "client"
	ExpiresAt  time.Time `json:"expiresAt"`
	DaysLeft   int       `json:"daysLeft"`
}

// StatusInfo is the wire format for the get-status response.
type StatusInfo struct {
	Running      bool          `json:"running"`
	DeviceCount  int           `json:"deviceCount"`
	KeyCount     int           `json:"keyCount"`
	SocketPath   string        `json:"socketPath"`
	CertWarnings []CertWarning `json:"certWarnings"`
}

// FullDeviceInfo is the wire format for a device with cert paths, used by get-device.
type FullDeviceInfo struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	TailscaleIP     string     `json:"tailscaleIp"`
	ListenPort      int        `json:"listenPort"`
	CertFingerprint string     `json:"certFingerprint"`
	CertPath        string     `json:"certPath"`
	ClientCertPath  string     `json:"clientCertPath"`
	ClientKeyPath   string     `json:"clientKeyPath"`
	LastSeen        *time.Time `json:"lastSeen,omitempty"`
	Source          string     `json:"source"`
}

// ControlServerConfig holds configuration for the control socket server.
type ControlServerConfig struct {
	SocketPath  string
	Registry    *Registry
	DevicesPath string
	KeyLister   func() []KeyInfo
}

// ControlServer listens on a Unix socket for line-delimited JSON commands.
type ControlServer struct {
	socketPath  string
	registry    *Registry
	devicesPath string
	keyLister   func() []KeyInfo
	listener    net.Listener
	wg          sync.WaitGroup
	done        chan struct{}
	stopOnce    sync.Once
}

// NewControlServer creates a new control socket server.
func NewControlServer(cfg ControlServerConfig) *ControlServer {
	return &ControlServer{
		socketPath:  cfg.SocketPath,
		registry:    cfg.Registry,
		devicesPath: cfg.DevicesPath,
		keyLister:   cfg.KeyLister,
		done:        make(chan struct{}),
	}
}

// Start begins listening on the Unix socket for control commands.
func (s *ControlServer) Start() error {
	// Remove stale socket file.
	os.Remove(s.socketPath)

	if err := os.MkdirAll(filepath.Dir(s.socketPath), 0700); err != nil {
		return fmt.Errorf("create control socket dir: %w", err)
	}

	ln, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listen on control socket %s: %w", s.socketPath, err)
	}

	if err := os.Chmod(s.socketPath, 0600); err != nil {
		ln.Close()
		return fmt.Errorf("chmod control socket: %w", err)
	}

	s.listener = ln
	s.wg.Add(1)
	go s.acceptLoop()
	return nil
}

// Stop shuts down the control socket server.
func (s *ControlServer) Stop() {
	s.stopOnce.Do(func() {
		close(s.done)
		if s.listener != nil {
			s.listener.Close()
		}
	})
	s.wg.Wait()
}

func (s *ControlServer) acceptLoop() {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.done:
				return
			default:
				continue
			}
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConn(conn)
		}()
	}
}

func (s *ControlServer) handleConn(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))

	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return
	}

	var req Request
	if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
		s.writeResponse(conn, Response{Status: "error", Error: "invalid JSON"})
		return
	}

	resp := s.handleCommand(req)
	s.writeResponse(conn, resp)
}

func (s *ControlServer) writeResponse(conn net.Conn, resp Response) {
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	fmt.Fprintf(conn, "%s\n", data)
}

func (s *ControlServer) handleCommand(req Request) Response {
	switch req.Command {
	case "register-device":
		return s.handleRegisterDevice(req)
	case "list-devices":
		return s.handleListDevices()
	case "revoke-device":
		return s.handleRevokeDevice(req)
	case "get-status":
		return s.handleGetStatus()
	case "get-keys":
		return s.handleGetKeys()
	case "get-device":
		return s.handleGetDevice(req)
	default:
		return Response{Status: "error", Error: fmt.Sprintf("unknown command: %s", req.Command)}
	}
}

func (s *ControlServer) handleRegisterDevice(req Request) Response {
	if req.DeviceID == "" {
		return Response{Status: "error", Error: "deviceId is required"}
	}

	// Reload devices from disk and merge into registry.
	devices, err := LoadDevicesFromJSON(s.devicesPath)
	if err != nil {
		return Response{Status: "error", Error: fmt.Sprintf("reload devices: %v", err)}
	}

	// Re-merge: keep existing nix-declared, overlay with new runtime devices.
	var nixDevices []Device
	var runtimeDevices []Device

	// Collect current nix-declared devices from registry.
	for _, dev := range s.registry.ListAll() {
		if dev.Source == SourceNixDeclared {
			nixDevices = append(nixDevices, dev)
		}
	}

	// Use the freshly loaded runtime devices from disk.
	runtimeDevices = devices
	s.registry.Merge(nixDevices, runtimeDevices)

	return Response{Status: "ok"}
}

func (s *ControlServer) handleListDevices() Response {
	devices := s.registry.ListAll()
	infos := make([]DeviceInfo, 0, len(devices))
	for _, d := range devices {
		infos = append(infos, DeviceInfo{
			ID:              d.ID,
			Name:            d.Name,
			TailscaleIP:     d.TailscaleIP,
			ListenPort:      d.ListenPort,
			CertFingerprint: d.CertFingerprint,
			LastSeen:        d.LastSeen,
			Source:          string(d.Source),
		})
	}
	return Response{Status: "ok", Data: infos}
}

func (s *ControlServer) handleRevokeDevice(req Request) Response {
	if req.DeviceID == "" {
		return Response{Status: "error", Error: "deviceId is required"}
	}

	dev, ok := s.registry.Get(req.DeviceID)
	if !ok {
		return Response{Status: "error", Error: fmt.Sprintf("device %s not found", req.DeviceID)}
	}

	if dev.Source == SourceNixDeclared {
		return Response{Status: "error", Error: "cannot revoke nix-declared device; remove it from your NixOS configuration"}
	}

	// Delete cert files from disk.
	deleteCertFiles(dev)

	s.registry.Remove(req.DeviceID)

	// Persist removal.
	if err := s.registry.SaveToJSON(s.devicesPath); err != nil {
		return Response{Status: "error", Error: fmt.Sprintf("save devices: %v", err)}
	}

	return Response{Status: "ok"}
}

// deleteCertFiles removes the device's cert files and their parent directory.
// Errors are silently ignored (best-effort cleanup).
func deleteCertFiles(dev Device) {
	paths := []string{dev.CertPath, dev.ClientCertPath, dev.ClientKeyPath}
	var parentDir string
	for _, p := range paths {
		if p == "" {
			continue
		}
		os.Remove(p)
		if parentDir == "" {
			parentDir = filepath.Dir(p)
		}
	}
	// Remove parent directory if empty (cert subdirectory).
	if parentDir != "" {
		os.Remove(parentDir) // fails silently if not empty
	}
}

func (s *ControlServer) handleGetStatus() Response {
	devices := s.registry.ListAll()
	keyCount := 0
	if s.keyLister != nil {
		keyCount = len(s.keyLister())
	}

	warnings := collectCertWarnings(devices, time.Now(), 30)

	return Response{Status: "ok", Data: StatusInfo{
		Running:      true,
		DeviceCount:  len(devices),
		KeyCount:     keyCount,
		SocketPath:   s.socketPath,
		CertWarnings: warnings,
	}}
}

// collectCertWarnings checks cert files for each device and returns warnings
// for certs expiring within the given threshold in days.
func collectCertWarnings(devices []Device, now time.Time, thresholdDays int) []CertWarning {
	threshold := now.Add(time.Duration(thresholdDays) * 24 * time.Hour)
	var warnings []CertWarning

	for _, dev := range devices {
		// Check server cert (phone cert).
		if dev.CertPath != "" {
			if expiry, err := parseCertExpiry(dev.CertPath); err == nil {
				if expiry.Before(threshold) {
					daysLeft := int(expiry.Sub(now).Hours() / 24)
					if daysLeft < 0 {
						daysLeft = 0
					}
					warnings = append(warnings, CertWarning{
						DeviceID:   dev.ID,
						DeviceName: dev.Name,
						CertType:   "server",
						ExpiresAt:  expiry,
						DaysLeft:   daysLeft,
					})
				}
			}
		}

		// Check client cert (host cert).
		if dev.ClientCertPath != "" {
			if expiry, err := parseCertExpiry(dev.ClientCertPath); err == nil {
				if expiry.Before(threshold) {
					daysLeft := int(expiry.Sub(now).Hours() / 24)
					if daysLeft < 0 {
						daysLeft = 0
					}
					warnings = append(warnings, CertWarning{
						DeviceID:   dev.ID,
						DeviceName: dev.Name,
						CertType:   "client",
						ExpiresAt:  expiry,
						DaysLeft:   daysLeft,
					})
				}
			}
		}
	}

	if warnings == nil {
		warnings = []CertWarning{}
	}
	return warnings
}

// parseCertExpiry reads a PEM certificate file and returns its NotAfter time.
func parseCertExpiry(certPath string) (time.Time, error) {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return time.Time{}, err
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return time.Time{}, fmt.Errorf("no PEM block found in %s", certPath)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, err
	}

	return cert.NotAfter, nil
}

func (s *ControlServer) handleGetDevice(req Request) Response {
	if req.DeviceID == "" {
		return Response{Status: "error", Error: "deviceId is required"}
	}

	dev, ok := s.registry.Get(req.DeviceID)
	if !ok {
		return Response{Status: "error", Error: fmt.Sprintf("device %s not found", req.DeviceID)}
	}

	return Response{Status: "ok", Data: FullDeviceInfo{
		ID:              dev.ID,
		Name:            dev.Name,
		TailscaleIP:     dev.TailscaleIP,
		ListenPort:      dev.ListenPort,
		CertFingerprint: dev.CertFingerprint,
		CertPath:        dev.CertPath,
		ClientCertPath:  dev.ClientCertPath,
		ClientKeyPath:   dev.ClientKeyPath,
		LastSeen:        dev.LastSeen,
		Source:          string(dev.Source),
	}}
}

func (s *ControlServer) handleGetKeys() Response {
	if s.keyLister == nil {
		return Response{Status: "ok", Data: []KeyInfo{}}
	}
	keys := s.keyLister()
	if keys == nil {
		keys = []KeyInfo{}
	}
	return Response{Status: "ok", Data: keys}
}

// ControlClient sends commands to a running daemon's control socket.
type ControlClient struct {
	socketPath string
}

// NewControlClient creates a client for the given control socket path.
func NewControlClient(socketPath string) *ControlClient {
	return &ControlClient{socketPath: socketPath}
}

// SendCommand sends a request to the daemon and returns the response.
func (c *ControlClient) SendCommand(req Request) (*Response, error) {
	conn, err := net.DialTimeout("unix", c.socketPath, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connect to control socket: %w", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	if _, err := fmt.Fprintf(conn, "%s\n", data); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}
		return nil, fmt.Errorf("no response from control socket")
	}

	var resp Response
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	return &resp, nil
}
