package pairing

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/phaedrus-raznikov/nix-key/internal/daemon"
)

// PairConfig configures the nix-key pair command.
type PairConfig struct {
	TailscaleInterface string
	CertExpiry         time.Duration
	OTELEndpoint       string
	AgeIdentityPath    string
	HostName           string
	DevicesPath        string
	CertsDir           string
	ControlSocketPath  string

	// PairInfoFile, if set, writes the QR payload JSON to this path
	// before displaying the QR code. Used by E2E tests to extract
	// the pairing token and port without decoding the terminal QR.
	PairInfoFile string

	// For testing: override the Tailscale interface resolution.
	// Returns the Tailscale IP for the given interface name.
	InterfaceResolver func(name string) (string, error)

	// For testing: override the user confirmation prompt.
	// Receives the pairing request and returns true for approved.
	ConfirmFunc func(req PairingRequest) bool

	// For testing: override age encryption.
	// Encrypts plaintext using the given identity file path.
	Encryptor func(plaintext []byte, identityPath string) ([]byte, error)

	Stdout io.Writer
	Stdin  io.Reader
}

func (c *PairConfig) setDefaults() {
	if c.TailscaleInterface == "" {
		c.TailscaleInterface = "tailscale0"
	}
	if c.CertExpiry <= 0 {
		c.CertExpiry = 365 * 24 * time.Hour
	}
	if c.HostName == "" {
		h, err := os.Hostname()
		if err != nil {
			h = "unknown"
		}
		c.HostName = h
	}
	if c.DevicesPath == "" {
		home, _ := os.UserHomeDir()
		c.DevicesPath = filepath.Join(home, ".local", "state", "nix-key", "devices.json")
	}
	if c.CertsDir == "" {
		home, _ := os.UserHomeDir()
		c.CertsDir = filepath.Join(home, ".local", "state", "nix-key", "certs")
	}
	if c.AgeIdentityPath == "" {
		home, _ := os.UserHomeDir()
		c.AgeIdentityPath = filepath.Join(home, ".local", "state", "nix-key", "age-identity.txt")
	}
	if c.InterfaceResolver == nil {
		c.InterfaceResolver = getTailscaleIP
	}
	if c.Encryptor == nil {
		c.Encryptor = ageEncrypt
	}
	if c.Stdout == nil {
		c.Stdout = os.Stdout
	}
	if c.Stdin == nil {
		c.Stdin = os.Stdin
	}
}

// RunPair executes the full nix-key pair flow:
//  1. Check Tailscale interface is available (FR-E11)
//  2. Generate host client cert pair with configured expiry
//  3. Generate one-time pairing token
//  4. Start temporary HTTPS pairing server
//  5. Display QR code with pairing info
//  6. On phone connection: show device info, prompt user
//  7. On confirm: encrypt certs with age (FR-104), store device, notify daemon
func RunPair(ctx context.Context, cfg PairConfig) error {
	cfg.setDefaults()

	// Step 1: Check Tailscale interface (FR-E11)
	tsIP, err := cfg.InterfaceResolver(cfg.TailscaleInterface)
	if err != nil {
		return fmt.Errorf("pairing failed: %w", err)
	}

	// Step 2: Generate host client cert pair (FR-031)
	clientCertPEM, clientKeyPEM, err := generateClientCertPair(cfg.CertExpiry)
	if err != nil {
		return fmt.Errorf("generate host client cert: %w", err)
	}

	// Step 3: Generate one-time token
	token, err := generateToken()
	if err != nil {
		return fmt.Errorf("generate pairing token: %w", err)
	}

	// Step 4: Create and start pairing server
	server, err := NewPairingServer(PairingServerConfig{
		Token:          token,
		HostName:       cfg.HostName,
		HostClientCert: string(clientCertPEM),
	})
	if err != nil {
		return fmt.Errorf("create pairing server: %w", err)
	}

	// Set up confirmation callback
	if cfg.ConfirmFunc != nil {
		server.SetConfirmCallback(cfg.ConfirmFunc)
	} else {
		server.SetConfirmCallback(func(req PairingRequest) bool {
			return promptConfirm(cfg.Stdout, cfg.Stdin, req)
		})
	}

	// Bind to Tailscale interface
	ln, err := tls.Listen("tcp", tsIP+":0", server.TLSConfig())
	if err != nil {
		return fmt.Errorf("listen on tailscale interface: %w", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port

	// Step 5: Display QR code (FR-070)
	qrParams := QRParams{
		Host:         tsIP,
		Port:         port,
		Cert:         server.ServerCertPEM(),
		Token:        token,
		OTELEndpoint: cfg.OTELEndpoint,
	}

	// Write pairing info to file for E2E test automation.
	if cfg.PairInfoFile != "" {
		infoJSON, err := json.Marshal(qrParams)
		if err != nil {
			return fmt.Errorf("marshal pair info: %w", err)
		}
		if err := os.WriteFile(cfg.PairInfoFile, infoJSON, 0600); err != nil {
			return fmt.Errorf("write pair info file: %w", err)
		}
	}

	qrStr, err := RenderQR(qrParams)
	if err != nil {
		return fmt.Errorf("render QR code: %w", err)
	}

	fmt.Fprintf(cfg.Stdout, "\n=== nix-key pairing ===\n")
	fmt.Fprintf(cfg.Stdout, "Host: %s (%s)\n", cfg.HostName, tsIP)
	fmt.Fprintf(cfg.Stdout, "Listening on port %d\n", port)
	if cfg.OTELEndpoint != "" {
		fmt.Fprintf(cfg.Stdout, "OTEL endpoint: %s\n", cfg.OTELEndpoint)
	}
	fmt.Fprintf(cfg.Stdout, "\nScan this QR code with the nix-key app:\n\n")
	fmt.Fprint(cfg.Stdout, qrStr)
	fmt.Fprintf(cfg.Stdout, "\nWaiting for phone to connect...\n")

	// Step 6: Serve pairing requests (blocks until completed or ctx cancelled)
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Serve(ln)
	}()

	// Wait for either context cancellation or server completion
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
		return ctx.Err()
	case err := <-serverErr:
		// Server stopped - check if pairing was completed
		if err != nil && !isServerClosed(err) {
			return fmt.Errorf("pairing server: %w", err)
		}
	}

	// Check if we have a completed request
	completedReq := server.GetCompletedRequest()
	if completedReq == nil {
		fmt.Fprintf(cfg.Stdout, "\nPairing was not completed.\n")
		return nil
	}

	// Step 7: On confirm - process pairing result
	fmt.Fprintf(cfg.Stdout, "\nPairing approved! Processing...\n")

	if err := processPairingResult(cfg, completedReq, clientCertPEM, clientKeyPEM); err != nil {
		return fmt.Errorf("process pairing result: %w", err)
	}

	fmt.Fprintf(cfg.Stdout, "Device %q paired successfully.\n", completedReq.PhoneName)
	return nil
}

// processPairingResult handles post-approval work: encrypt certs, store device, notify daemon.
func processPairingResult(cfg PairConfig, req *PairingRequest, clientCertPEM, clientKeyPEM []byte) error {
	// Compute device ID from phone's server cert fingerprint
	certFP, err := certFingerprint([]byte(req.ServerCert))
	if err != nil {
		return fmt.Errorf("compute cert fingerprint: %w", err)
	}
	deviceID := certFP

	// Create certs directory
	deviceCertDir := filepath.Join(cfg.CertsDir, deviceID[:16])
	if err := os.MkdirAll(deviceCertDir, 0700); err != nil {
		return fmt.Errorf("create cert directory: %w", err)
	}

	// Ensure age identity exists
	if err := ensureAgeIdentity(cfg.AgeIdentityPath); err != nil {
		return fmt.Errorf("ensure age identity: %w", err)
	}

	// Save phone's server cert (plaintext - it's a public cert)
	phoneCertPath := filepath.Join(deviceCertDir, "phone-server-cert.pem")
	if err := os.WriteFile(phoneCertPath, []byte(req.ServerCert), 0600); err != nil {
		return fmt.Errorf("write phone cert: %w", err)
	}

	// Save host client cert (plaintext - it's a public cert)
	hostCertPath := filepath.Join(deviceCertDir, "host-client-cert.pem")
	if err := os.WriteFile(hostCertPath, clientCertPEM, 0600); err != nil {
		return fmt.Errorf("write host client cert: %w", err)
	}

	// Encrypt and save host client key (FR-104)
	encryptedKey, err := cfg.Encryptor(clientKeyPEM, cfg.AgeIdentityPath)
	if err != nil {
		return fmt.Errorf("encrypt client key: %w", err)
	}
	hostKeyPath := filepath.Join(deviceCertDir, "host-client-key.pem.age")
	if err := os.WriteFile(hostKeyPath, encryptedKey, 0600); err != nil {
		return fmt.Errorf("write encrypted host client key: %w", err)
	}

	// Register device in devices.json
	device := daemon.Device{
		ID:              deviceID,
		Name:            req.PhoneName,
		TailscaleIP:     req.TailscaleIP,
		ListenPort:      req.ListenPort,
		CertFingerprint: certFP,
		CertPath:        phoneCertPath,
		ClientCertPath:  hostCertPath,
		ClientKeyPath:   hostKeyPath,
		Source:          daemon.SourceRuntimePaired,
	}

	// Load existing devices, add new one, save
	existing, err := daemon.LoadDevicesFromJSON(cfg.DevicesPath)
	if err != nil {
		return fmt.Errorf("load devices: %w", err)
	}

	reg := daemon.NewRegistry()
	reg.Merge(nil, existing)
	reg.Add(device)

	if err := reg.SaveToJSON(cfg.DevicesPath); err != nil {
		return fmt.Errorf("save devices: %w", err)
	}

	// Notify daemon via control socket (best-effort, T026 implements full control socket)
	if cfg.ControlSocketPath != "" {
		if err := notifyDaemon(cfg.ControlSocketPath, device); err != nil {
			// Non-fatal: daemon might not be running
			fmt.Fprintf(cfg.Stdout, "Warning: could not notify daemon: %v\n", err)
		}
	}

	return nil
}

// getTailscaleIP resolves the Tailscale IP from a network interface name.
func getTailscaleIP(ifaceName string) (string, error) {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return "", fmt.Errorf("tailscale interface %q unavailable: %w", ifaceName, err)
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return "", fmt.Errorf("tailscale interface %q: get addresses: %w", ifaceName, err)
	}

	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip != nil && ip.To4() != nil {
			return ip.String(), nil
		}
	}

	return "", fmt.Errorf("tailscale interface %q has no IPv4 addresses", ifaceName)
}

// generateClientCertPair creates a self-signed ECDSA-P256 client certificate
// for mTLS connections from host to phone (FR-031).
func generateClientCertPair(expiry time.Duration) (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate key: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("generate serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: "nix-key host client",
		},
		NotBefore:             time.Now().Add(-5 * time.Minute),
		NotAfter:              time.Now().Add(expiry),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("create certificate: %w", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal key: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

// generateToken creates a cryptographically random one-time pairing token.
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// certFingerprint computes the SHA256 fingerprint of a PEM-encoded certificate.
func certFingerprint(certPEM []byte) (string, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return "", fmt.Errorf("failed to decode PEM certificate")
	}
	hash := sha256.Sum256(block.Bytes)
	return hex.EncodeToString(hash[:]), nil
}

// promptConfirm shows the phone's info and prompts for authorization (FR-025).
func promptConfirm(out io.Writer, in io.Reader, req PairingRequest) bool {
	fmt.Fprintf(out, "\n--- Phone wants to pair ---\n")
	fmt.Fprintf(out, "  Name:         %s\n", req.PhoneName)
	fmt.Fprintf(out, "  Tailscale IP: %s\n", req.TailscaleIP)
	fmt.Fprintf(out, "  Listen Port:  %d\n", req.ListenPort)
	fmt.Fprintf(out, "\nAuthorize? [y/N] ")

	scanner := bufio.NewScanner(in)
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		return answer == "y" || answer == "yes"
	}
	return false
}

// ensureAgeIdentity generates an age identity file if it doesn't exist.
func ensureAgeIdentity(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create identity directory: %w", err)
	}

	cmd := exec.Command("age-keygen", "-o", path)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("age-keygen: %w: %s", err, output)
	}

	return os.Chmod(path, 0600)
}

// ageEncrypt encrypts data using the age CLI with the public key from the identity file.
func ageEncrypt(plaintext []byte, identityPath string) ([]byte, error) {
	// Extract recipient public key from identity file
	recipient, err := extractAgeRecipient(identityPath)
	if err != nil {
		return nil, fmt.Errorf("extract age recipient: %w", err)
	}

	cmd := exec.Command("age", "-e", "-r", recipient)
	cmd.Stdin = strings.NewReader(string(plaintext))
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("age encrypt: %s", exitErr.Stderr)
		}
		return nil, fmt.Errorf("age encrypt: %w", err)
	}
	return out, nil
}

// extractAgeRecipient reads the public key from an age identity file.
// The public key is in a comment line: "# public key: age1..."
func extractAgeRecipient(identityPath string) (string, error) {
	f, err := os.Open(identityPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "# public key: ") {
			return strings.TrimPrefix(line, "# public key: "), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("no public key found in %s", identityPath)
}

// notifyDaemon sends a register-device command to the daemon's control socket.
func notifyDaemon(socketPath string, dev daemon.Device) error {
	client := daemon.NewControlClient(socketPath)
	resp, err := client.SendCommand(daemon.Request{
		Command:  "register-device",
		DeviceID: dev.ID,
	})
	if err != nil {
		return err
	}
	if resp.Status != "ok" {
		return fmt.Errorf("daemon: %s", resp.Error)
	}
	return nil
}

// isServerClosed checks if the error indicates the server was intentionally shut down.
func isServerClosed(err error) bool {
	return err != nil && strings.Contains(err.Error(), "Server closed")
}
