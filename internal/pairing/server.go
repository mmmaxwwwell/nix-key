package pairing

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"sync"
	"time"
)

// PairingRequest is the JSON body POSTed by the phone to the pairing endpoint.
type PairingRequest struct {
	PhoneName   string `json:"phoneName"`
	TailscaleIP string `json:"tailscaleIp"`
	ListenPort  int    `json:"listenPort"`
	ServerCert  string `json:"serverCert"`
	Token       string `json:"token"`
}

// PairingResponse is the JSON response from the pairing server.
type PairingResponse struct {
	HostName       string `json:"hostName,omitempty"`
	HostClientCert string `json:"hostClientCert,omitempty"`
	Status         string `json:"status"`
}

// ConfirmCallback is called when a phone connects to the pairing server.
// It receives the pairing request and should block until the user confirms
// or denies. Returns true for approved, false for denied.
type ConfirmCallback func(req PairingRequest) bool

// PairingServerConfig holds configuration for the temporary pairing server.
type PairingServerConfig struct {
	Token          string
	HostName       string
	HostClientCert string
	PairingTimeout time.Duration
}

// PairingServer is a temporary HTTPS server for the device pairing flow.
// It accepts a single pairing request, validates the token, invokes a
// confirmation callback, and responds with the host's info on approval.
type PairingServer struct {
	config    PairingServerConfig
	tlsCert   tls.Certificate
	serverPEM string // PEM-encoded server certificate for QR payload

	mu              sync.Mutex
	tokenUsed       bool
	confirmCallback ConfirmCallback
	completedReq    *PairingRequest

	httpServer *http.Server
}

// NewPairingServer creates a new pairing server with a self-signed TLS cert.
func NewPairingServer(cfg PairingServerConfig) (*PairingServer, error) {
	if cfg.Token == "" {
		return nil, fmt.Errorf("pairing server: token is required")
	}
	if cfg.HostName == "" {
		return nil, fmt.Errorf("pairing server: hostName is required")
	}
	if cfg.PairingTimeout <= 0 {
		cfg.PairingTimeout = 2 * time.Minute
	}

	tlsCert, certPEM, err := generateSelfSignedCert()
	if err != nil {
		return nil, fmt.Errorf("pairing server: generate cert: %w", err)
	}

	s := &PairingServer{
		config:    cfg,
		tlsCert:   tlsCert,
		serverPEM: certPEM,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/pair", s.handlePair)

	s.httpServer = &http.Server{
		Handler: mux,
	}

	return s, nil
}

// SetConfirmCallback sets the callback invoked when a phone requests pairing.
func (s *PairingServer) SetConfirmCallback(cb ConfirmCallback) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.confirmCallback = cb
}

// TLSConfig returns the TLS configuration for the pairing server.
func (s *PairingServer) TLSConfig() *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{s.tlsCert},
		MinVersion:   tls.VersionTLS13,
	}
}

// ServerCertPEM returns the PEM-encoded server certificate for inclusion
// in the QR code payload.
func (s *PairingServer) ServerCertPEM() string {
	return s.serverPEM
}

// Serve starts serving on the given listener. This blocks until the server
// is shut down.
func (s *PairingServer) Serve(ln net.Listener) error {
	return s.httpServer.Serve(ln)
}

// Shutdown gracefully shuts down the pairing server.
func (s *PairingServer) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// GetCompletedRequest returns the pairing request that was successfully
// approved, or nil if no pairing was completed.
func (s *PairingServer) GetCompletedRequest() *PairingRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.completedReq
}

func (s *PairingServer) handlePair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req PairingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.PhoneName == "" || req.TailscaleIP == "" || req.ListenPort == 0 || req.ServerCert == "" || req.Token == "" {
		http.Error(w, "missing required fields", http.StatusBadRequest)
		return
	}

	// Validate and consume one-time token (FR-027, FR-E10)
	s.mu.Lock()
	if s.tokenUsed || req.Token != s.config.Token {
		tokenWasUsed := s.tokenUsed
		s.mu.Unlock()
		if tokenWasUsed {
			writeJSON(w, http.StatusUnauthorized, PairingResponse{Status: "denied"})
		} else {
			writeJSON(w, http.StatusUnauthorized, PairingResponse{Status: "denied"})
		}
		return
	}
	s.tokenUsed = true
	cb := s.confirmCallback
	s.mu.Unlock()

	// If no callback, deny by default
	if cb == nil {
		writeJSON(w, http.StatusForbidden, PairingResponse{Status: "denied"})
		return
	}

	// Invoke confirm callback with timeout (FR-025, FR-028)
	type confirmResult struct {
		approved bool
	}
	resultCh := make(chan confirmResult, 1)
	go func() {
		approved := cb(req)
		resultCh <- confirmResult{approved: approved}
	}()

	var approved bool
	select {
	case result := <-resultCh:
		approved = result.approved
	case <-time.After(s.config.PairingTimeout):
		approved = false
	}

	if approved {
		s.mu.Lock()
		reqCopy := req
		s.completedReq = &reqCopy
		s.mu.Unlock()

		writeJSON(w, http.StatusOK, PairingResponse{
			HostName:       s.config.HostName,
			HostClientCert: s.config.HostClientCert,
			Status:         "approved",
		})
	} else {
		writeJSON(w, http.StatusForbidden, PairingResponse{Status: "denied"})
	}

	// Token is consumed (one-time use). Shut down the server so Serve() returns
	// and RunPair can proceed with post-pairing processing.
	go s.httpServer.Shutdown(context.Background())
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// generateSelfSignedCert creates a self-signed ECDSA-P256 certificate
// valid for 1 hour (sufficient for the temporary pairing server).
func generateSelfSignedCert() (tls.Certificate, string, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, "", fmt.Errorf("generate key: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, "", fmt.Errorf("generate serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: "nix-key pairing server",
		},
		NotBefore:             time.Now().Add(-5 * time.Minute), // clock skew tolerance
		NotAfter:              time.Now().Add(1 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, "", fmt.Errorf("create certificate: %w", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, "", fmt.Errorf("marshal key: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, "", fmt.Errorf("create TLS keypair: %w", err)
	}

	return tlsCert, string(certPEM), nil
}
