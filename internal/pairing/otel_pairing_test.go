package pairing

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestIntegrationPairingWithOTELConfig verifies FR-087/FR-088: when otelEndpoint
// is configured, the QR payload includes it. A simulated phone receives the
// endpoint during pairing and can store it.
func TestIntegrationPairingWithOTELConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	const otelEndpoint = "jaeger.example.com:4317"

	// Step 1: Generate host client cert and token.
	clientCertPEM, _, err := generateClientCertPair(24 * time.Hour)
	if err != nil {
		t.Fatalf("generate host client cert: %v", err)
	}
	token, err := generateToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	// Step 2: Create pairing server.
	pairSrv, err := NewPairingServer(PairingServerConfig{
		Token:          token,
		HostName:       "otel-test-host",
		HostClientCert: string(clientCertPEM),
		PairingTimeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("create pairing server: %v", err)
	}
	pairSrv.SetConfirmCallback(func(req PairingRequest) bool {
		return true // Auto-approve
	})

	ln, err := tls.Listen("tcp", "127.0.0.1:0", pairSrv.TLSConfig())
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go pairSrv.Serve(ln)
	defer pairSrv.Shutdown(context.Background())

	port := ln.Addr().(*net.TCPAddr).Port

	// Step 3: Generate QR payload with OTEL endpoint.
	qrParams := QRParams{
		Host:         "127.0.0.1",
		Port:         port,
		Cert:         pairSrv.ServerCertPEM(),
		Token:        token,
		OTELEndpoint: otelEndpoint,
	}
	payload, err := GenerateQRPayload(qrParams)
	if err != nil {
		t.Fatalf("GenerateQRPayload: %v", err)
	}

	// Step 4: Verify QR payload contains OTEL endpoint (simulates phone decoding QR).
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("base64 decode QR payload: %v", err)
	}

	var qrData map[string]interface{}
	if err := json.Unmarshal(decoded, &qrData); err != nil {
		t.Fatalf("unmarshal QR JSON: %v", err)
	}

	otelFromQR, ok := qrData["otel"]
	if !ok {
		t.Fatal("QR payload missing 'otel' field when OTELEndpoint is configured")
	}
	if otelFromQR != otelEndpoint {
		t.Errorf("QR otel = %q, want %q", otelFromQR, otelEndpoint)
	}

	// Step 5: Simulated phone POSTs to pairing endpoint (phone has decoded OTEL from QR).
	phoneCertPEM, _, err := generateClientCertPair(time.Hour)
	if err != nil {
		t.Fatalf("generate phone cert: %v", err)
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 10 * time.Second,
	}

	phoneReq := PairingRequest{
		PhoneName:   "OTEL Test Phone",
		TailscaleIP: "100.64.0.50",
		ListenPort:  29418,
		ServerCert:  string(phoneCertPEM),
		Token:       token,
	}
	body, _ := json.Marshal(phoneReq)

	resp, err := httpClient.Post(
		fmt.Sprintf("https://127.0.0.1:%d/pair", port),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("phone POST /pair: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, respBody)
	}

	var pairResp PairingResponse
	if err := json.NewDecoder(resp.Body).Decode(&pairResp); err != nil {
		t.Fatalf("decode pairing response: %v", err)
	}
	if pairResp.Status != "approved" {
		t.Fatalf("expected status=approved, got %q", pairResp.Status)
	}

	// Step 6: Verify the phone would store the OTEL endpoint.
	// Simulate the phone's storage: after successful pairing, phone checks
	// the QR-decoded otelEndpoint and stores it in EncryptedSharedPreferences.
	// In Go tests, we verify the endpoint was correctly propagated through the
	// QR payload by asserting it matches the original configured value.
	storedEndpoint := otelFromQR.(string)
	if storedEndpoint != otelEndpoint {
		t.Errorf("phone stored OTEL endpoint = %q, want %q", storedEndpoint, otelEndpoint)
	}

	// Verify pairing completed successfully
	completedReq := pairSrv.GetCompletedRequest()
	if completedReq == nil {
		t.Fatal("expected completed request from pairing server")
	}
	if completedReq.PhoneName != "OTEL Test Phone" {
		t.Errorf("completed phone name = %q, want %q", completedReq.PhoneName, "OTEL Test Phone")
	}
}

// TestIntegrationPairingWithoutOTEL verifies that when otelEndpoint is not
// configured, the QR payload omits the otel field entirely.
func TestIntegrationPairingWithoutOTEL(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	clientCertPEM, _, err := generateClientCertPair(24 * time.Hour)
	if err != nil {
		t.Fatalf("generate host client cert: %v", err)
	}
	token, err := generateToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	pairSrv, err := NewPairingServer(PairingServerConfig{
		Token:          token,
		HostName:       "no-otel-host",
		HostClientCert: string(clientCertPEM),
		PairingTimeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("create pairing server: %v", err)
	}
	pairSrv.SetConfirmCallback(func(req PairingRequest) bool {
		return true
	})

	ln, err := tls.Listen("tcp", "127.0.0.1:0", pairSrv.TLSConfig())
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go pairSrv.Serve(ln)
	defer pairSrv.Shutdown(context.Background())

	port := ln.Addr().(*net.TCPAddr).Port

	// QR payload without OTEL
	qrParams := QRParams{
		Host:  "127.0.0.1",
		Port:  port,
		Cert:  pairSrv.ServerCertPEM(),
		Token: token,
		// OTELEndpoint intentionally empty
	}
	payload, err := GenerateQRPayload(qrParams)
	if err != nil {
		t.Fatalf("GenerateQRPayload: %v", err)
	}

	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}

	var qrData map[string]interface{}
	if err := json.Unmarshal(decoded, &qrData); err != nil {
		t.Fatalf("unmarshal QR JSON: %v", err)
	}

	// otel field should NOT be present (omitempty)
	if _, ok := qrData["otel"]; ok {
		t.Error("QR payload should not contain 'otel' field when endpoint is not configured")
	}

	// Pairing should still work without OTEL
	phoneCertPEM, _, err := generateClientCertPair(time.Hour)
	if err != nil {
		t.Fatalf("generate phone cert: %v", err)
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 10 * time.Second,
	}

	phoneReq := PairingRequest{
		PhoneName:   "No-OTEL Phone",
		TailscaleIP: "100.64.0.60",
		ListenPort:  29418,
		ServerCert:  string(phoneCertPEM),
		Token:       token,
	}
	body, _ := json.Marshal(phoneReq)

	resp, err := httpClient.Post(
		fmt.Sprintf("https://127.0.0.1:%d/pair", port),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("phone POST /pair: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, respBody)
	}
}

// TestPairInfoFileIncludesOTEL verifies that the --pair-info-file output
// includes the OTEL endpoint when configured (used by E2E tests).
func TestPairInfoFileIncludesOTEL(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	dir := t.TempDir()
	pairInfoPath := filepath.Join(dir, "pair-info.json")

	var output safeBuffer
	cfg := PairConfig{
		TailscaleInterface: "loopback",
		CertExpiry:         24 * time.Hour,
		HostName:           "info-file-host",
		OTELEndpoint:       "collector.local:4317",
		PairInfoFile:       pairInfoPath,
		Stdout:             &output,
		Stdin:              strings.NewReader(""),
		InterfaceResolver: func(name string) (string, error) {
			return "127.0.0.1", nil
		},
		ConfirmFunc: func(req PairingRequest) bool {
			return false
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go RunPair(ctx, cfg)

	// Wait for pair info file to be written
	var infoData []byte
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		data, err := os.ReadFile(pairInfoPath)
		if err == nil && len(data) > 0 {
			infoData = data
			break
		}
	}

	if len(infoData) == 0 {
		t.Fatal("pair info file was not written")
	}

	var info QRParams
	if err := json.Unmarshal(infoData, &info); err != nil {
		t.Fatalf("unmarshal pair info: %v", err)
	}

	if info.OTELEndpoint != "collector.local:4317" {
		t.Errorf("pair info OTEL = %q, want %q", info.OTELEndpoint, "collector.local:4317")
	}
	if info.Host != "127.0.0.1" {
		t.Errorf("pair info host = %q, want %q", info.Host, "127.0.0.1")
	}

	cancel()
}
