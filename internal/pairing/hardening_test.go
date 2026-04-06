package pairing

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// T-HI-16: Concurrent pairing rejection — second connection gets denied
// because the one-time token is consumed by the first.
func TestIntegrationConcurrentPairingRejection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	token := "test-token-concurrent"
	server, err := NewPairingServer(PairingServerConfig{
		Token:          token,
		HostName:       "test-host",
		HostClientCert: "FAKE-CERT-PEM",
		PairingTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	// Auto-approve the first pairing.
	server.SetConfirmCallback(func(req PairingRequest) bool {
		return true
	})

	// Start TLS listener.
	ln, err := tls.Listen("tcp", "127.0.0.1:0", server.TLSConfig())
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() { _ = server.Serve(ln) }()

	addr := ln.Addr().String()

	// Build a pairing request payload.
	makePayload := func(phoneName string) []byte {
		req := PairingRequest{
			PhoneName:   phoneName,
			TailscaleIP: "100.64.0.1",
			ListenPort:  29418,
			ServerCert:  "-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----\n",
			Token:       token,
		}
		data, _ := json.Marshal(req)
		return data
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 10 * time.Second,
	}

	// Send two concurrent requests.
	type result struct {
		name   string
		status int
		body   string
		err    error
	}
	results := make(chan result, 2)

	var wg sync.WaitGroup
	for _, name := range []string{"Phone-A", "Phone-B"} {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			payload := makePayload(n)
			resp, err := httpClient.Post(
				fmt.Sprintf("https://%s/pair", addr),
				"application/json",
				bytes.NewReader(payload),
			)
			if err != nil {
				results <- result{name: n, err: err}
				return
			}
			defer resp.Body.Close()
			var pr PairingResponse
			_ = json.NewDecoder(resp.Body).Decode(&pr)
			results <- result{name: n, status: resp.StatusCode, body: pr.Status}
		}(name)
	}
	wg.Wait()
	close(results)

	approvedCount := 0
	deniedCount := 0
	for r := range results {
		if r.err != nil {
			// Connection errors after server shutdown are acceptable for the second request.
			deniedCount++
			continue
		}
		if r.status == http.StatusOK && r.body == "approved" {
			approvedCount++
		} else {
			deniedCount++
		}
	}

	if approvedCount != 1 {
		t.Errorf("expected exactly 1 approved, got %d", approvedCount)
	}
	if deniedCount != 1 {
		t.Errorf("expected exactly 1 denied/failed, got %d", deniedCount)
	}

	// Verify only one completed request stored.
	completed := server.GetCompletedRequest()
	if completed == nil {
		t.Fatal("expected one completed pairing request")
	}
}

// T-HI-21: Pairing without Tailscale interface fails immediately with a clear error.
func TestIntegrationPairingWithoutTailscale(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()

	cfg := PairConfig{
		TailscaleInterface: "nonexistent-iface-999",
		DevicesPath:        dir + "/devices.json",
		CertsDir:           dir + "/certs",
		AgeIdentityPath:    dir + "/age-identity.txt",
		HostName:           "test-host",
		// Use a mock resolver that simulates missing Tailscale interface.
		InterfaceResolver: func(name string) (string, error) {
			return "", fmt.Errorf("tailscale interface %q unavailable: no such network interface", name)
		},
		ConfirmFunc: func(req PairingRequest) bool { return false },
		Stdout:      &strings.Builder{},
		Stdin:       strings.NewReader(""),
	}

	err := RunPair(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error when Tailscale interface unavailable")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "tailscale interface") {
		t.Errorf("error should mention tailscale interface, got: %v", err)
	}
	if !strings.Contains(errMsg, "unavailable") {
		t.Errorf("error should mention unavailable, got: %v", err)
	}

	// Verify no pairing cert files were created.
	certsDir := dir + "/certs"
	if _, err := os.Stat(certsDir); err == nil {
		entries, _ := os.ReadDir(certsDir)
		if len(entries) > 0 {
			t.Error("certs directory should be empty after failed pairing")
		}
	}
}
