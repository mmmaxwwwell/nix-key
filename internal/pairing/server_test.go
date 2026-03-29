package pairing

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestPairingServerAcceptAndApprove(t *testing.T) {
	// Start pairing server with a callback that auto-approves.
	token := "test-token-abc123"
	hostName := "nixos-desktop"
	hostClientCert := "-----BEGIN CERTIFICATE-----\nHOSTCLIENTCERT\n-----END CERTIFICATE-----"

	srv, err := NewPairingServer(PairingServerConfig{
		Token:          token,
		HostName:       hostName,
		HostClientCert: hostClientCert,
		PairingTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewPairingServer: %v", err)
	}

	// Auto-approve callback
	srv.SetConfirmCallback(func(req PairingRequest) bool {
		return true
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()

	tlsLn := tls.NewListener(ln, srv.TLSConfig())
	go srv.Serve(tlsLn)
	defer srv.Shutdown(context.Background())

	// Simulate phone POST
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
		Timeout: 5 * time.Second,
	}

	reqBody := PairingRequest{
		PhoneName:   "Pixel 8",
		TailscaleIP: "100.64.0.5",
		ListenPort:  29418,
		ServerCert:  "-----BEGIN CERTIFICATE-----\nPHONECERT\n-----END CERTIFICATE-----",
		Token:       token,
	}
	body, _ := json.Marshal(reqBody)

	resp, err := client.Post(fmt.Sprintf("https://%s/pair", addr), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /pair: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, respBody)
	}

	var pairingResp PairingResponse
	if err := json.NewDecoder(resp.Body).Decode(&pairingResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if pairingResp.Status != "approved" {
		t.Errorf("expected status=approved, got %q", pairingResp.Status)
	}
	if pairingResp.HostName != hostName {
		t.Errorf("expected hostName=%q, got %q", hostName, pairingResp.HostName)
	}
	if pairingResp.HostClientCert != hostClientCert {
		t.Errorf("expected hostClientCert set, got %q", pairingResp.HostClientCert)
	}
}

func TestPairingServerDenied(t *testing.T) {
	token := "deny-token"
	srv, err := NewPairingServer(PairingServerConfig{
		Token:          token,
		HostName:       "nixos-host",
		HostClientCert: "CERT",
		PairingTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewPairingServer: %v", err)
	}

	// Deny callback
	srv.SetConfirmCallback(func(req PairingRequest) bool {
		return false
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()

	tlsLn := tls.NewListener(ln, srv.TLSConfig())
	go srv.Serve(tlsLn)
	defer srv.Shutdown(context.Background())

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 5 * time.Second,
	}

	reqBody := PairingRequest{
		PhoneName:   "Pixel 7",
		TailscaleIP: "100.64.0.6",
		ListenPort:  29418,
		ServerCert:  "PHONECERT",
		Token:       token,
	}
	body, _ := json.Marshal(reqBody)

	resp, err := client.Post(fmt.Sprintf("https://%s/pair", addr), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /pair: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}

	var pairingResp PairingResponse
	if err := json.NewDecoder(resp.Body).Decode(&pairingResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if pairingResp.Status != "denied" {
		t.Errorf("expected status=denied, got %q", pairingResp.Status)
	}
}

func TestPairingServerInvalidToken(t *testing.T) {
	srv, err := NewPairingServer(PairingServerConfig{
		Token:          "real-token",
		HostName:       "host",
		HostClientCert: "CERT",
		PairingTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewPairingServer: %v", err)
	}

	srv.SetConfirmCallback(func(req PairingRequest) bool {
		t.Error("confirm callback should not be called for invalid token")
		return false
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()

	tlsLn := tls.NewListener(ln, srv.TLSConfig())
	go srv.Serve(tlsLn)
	defer srv.Shutdown(context.Background())

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 5 * time.Second,
	}

	reqBody := PairingRequest{
		PhoneName:   "Pixel",
		TailscaleIP: "100.64.0.7",
		ListenPort:  29418,
		ServerCert:  "CERT",
		Token:       "wrong-token",
	}
	body, _ := json.Marshal(reqBody)

	resp, err := client.Post(fmt.Sprintf("https://%s/pair", addr), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /pair: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestPairingServerTokenReplay(t *testing.T) {
	token := "one-time-token"
	srv, err := NewPairingServer(PairingServerConfig{
		Token:          token,
		HostName:       "host",
		HostClientCert: "CERT",
		PairingTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewPairingServer: %v", err)
	}

	srv.SetConfirmCallback(func(req PairingRequest) bool {
		return true
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()

	tlsLn := tls.NewListener(ln, srv.TLSConfig())
	go srv.Serve(tlsLn)
	defer srv.Shutdown(context.Background())

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 5 * time.Second,
	}

	reqBody := PairingRequest{
		PhoneName:   "Pixel",
		TailscaleIP: "100.64.0.7",
		ListenPort:  29418,
		ServerCert:  "CERT",
		Token:       token,
	}
	body, _ := json.Marshal(reqBody)

	// First request should succeed
	resp, err := client.Post(fmt.Sprintf("https://%s/pair", addr), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("first POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", resp.StatusCode)
	}

	// Wait briefly for the server to shut down (it auto-shuts after token consumption).
	time.Sleep(100 * time.Millisecond)

	// Second request with same token should be rejected (FR-E10, FR-027).
	// The server shuts down after consuming the one-time token, so the replay
	// attempt either gets a 401 (if the server hasn't fully stopped yet) or a
	// connection refused (if it has). Both prevent token reuse.
	body2, _ := json.Marshal(reqBody)
	resp2, err := client.Post(fmt.Sprintf("https://%s/pair", addr), "application/json", bytes.NewReader(body2))
	if err != nil {
		// Connection refused means server is shut down — replay is rejected.
		return
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("second request (replay): expected 401 or connection refused, got %d", resp2.StatusCode)
	}
}

func TestPairingServerTimeout(t *testing.T) {
	token := "timeout-token"
	srv, err := NewPairingServer(PairingServerConfig{
		Token:          token,
		HostName:       "host",
		HostClientCert: "CERT",
		PairingTimeout: 1 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewPairingServer: %v", err)
	}

	// Callback that blocks longer than timeout — simulates user not responding
	srv.SetConfirmCallback(func(req PairingRequest) bool {
		time.Sleep(3 * time.Second)
		return true
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()

	tlsLn := tls.NewListener(ln, srv.TLSConfig())
	go srv.Serve(tlsLn)
	defer srv.Shutdown(context.Background())

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 5 * time.Second,
	}

	reqBody := PairingRequest{
		PhoneName:   "Pixel",
		TailscaleIP: "100.64.0.7",
		ListenPort:  29418,
		ServerCert:  "CERT",
		Token:       token,
	}
	body, _ := json.Marshal(reqBody)

	resp, err := client.Post(fmt.Sprintf("https://%s/pair", addr), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	// Should get denied due to timeout
	var pairingResp PairingResponse
	if err := json.NewDecoder(resp.Body).Decode(&pairingResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if pairingResp.Status != "denied" {
		t.Errorf("expected status=denied on timeout, got %q", pairingResp.Status)
	}
}

func TestPairingServerMissingFields(t *testing.T) {
	srv, err := NewPairingServer(PairingServerConfig{
		Token:          "token",
		HostName:       "host",
		HostClientCert: "CERT",
		PairingTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewPairingServer: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()

	tlsLn := tls.NewListener(ln, srv.TLSConfig())
	go srv.Serve(tlsLn)
	defer srv.Shutdown(context.Background())

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 5 * time.Second,
	}

	// Missing phoneName
	reqBody := PairingRequest{
		PhoneName:   "",
		TailscaleIP: "100.64.0.7",
		ListenPort:  29418,
		ServerCert:  "CERT",
		Token:       "token",
	}
	body, _ := json.Marshal(reqBody)

	resp, err := client.Post(fmt.Sprintf("https://%s/pair", addr), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing fields, got %d", resp.StatusCode)
	}
}

func TestPairingServerCertAvailable(t *testing.T) {
	srv, err := NewPairingServer(PairingServerConfig{
		Token:          "token",
		HostName:       "host",
		HostClientCert: "CERT",
		PairingTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewPairingServer: %v", err)
	}

	tlsCfg := srv.TLSConfig()
	if len(tlsCfg.Certificates) == 0 {
		t.Fatal("TLS config has no certificates")
	}

	// Verify the generated cert is parseable
	cert, err := x509.ParseCertificate(tlsCfg.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatalf("parse generated cert: %v", err)
	}

	if cert.NotAfter.Before(time.Now()) {
		t.Error("generated cert is already expired")
	}
}

func TestPairingServerGetCompletedRequest(t *testing.T) {
	token := "complete-token"
	srv, err := NewPairingServer(PairingServerConfig{
		Token:          token,
		HostName:       "host",
		HostClientCert: "CERT",
		PairingTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewPairingServer: %v", err)
	}

	srv.SetConfirmCallback(func(req PairingRequest) bool {
		return true
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()

	tlsLn := tls.NewListener(ln, srv.TLSConfig())
	go srv.Serve(tlsLn)
	defer srv.Shutdown(context.Background())

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 5 * time.Second,
	}

	reqBody := PairingRequest{
		PhoneName:   "Pixel 8",
		TailscaleIP: "100.64.0.5",
		ListenPort:  29418,
		ServerCert:  "PHONECERT",
		Token:       token,
	}
	body, _ := json.Marshal(reqBody)

	resp, err := client.Post(fmt.Sprintf("https://%s/pair", addr), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()

	// After successful pairing, GetCompletedRequest should return the request
	completed := srv.GetCompletedRequest()
	if completed == nil {
		t.Fatal("expected completed request, got nil")
	}
	if completed.PhoneName != "Pixel 8" {
		t.Errorf("expected phoneName=Pixel 8, got %q", completed.PhoneName)
	}
}

func TestPairingServerWrongMethod(t *testing.T) {
	srv, err := NewPairingServer(PairingServerConfig{
		Token:          "token",
		HostName:       "host",
		HostClientCert: "CERT",
		PairingTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewPairingServer: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()

	tlsLn := tls.NewListener(ln, srv.TLSConfig())
	go srv.Serve(tlsLn)
	defer srv.Shutdown(context.Background())

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(fmt.Sprintf("https://%s/pair", addr))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.StatusCode)
	}
}
