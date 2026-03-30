package agent_test

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	nixkeyv1 "github.com/phaedrus-raznikov/nix-key/gen/nixkey/v1"
)

// TestIntegrationE2ESignLatency starts an SSH agent backed by an in-process
// gRPC phone server with auto-approve, runs 20 sign requests, and asserts
// p95 latency < 2s.
func TestIntegrationE2ESignLatency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping latency test in short mode")
	}

	_, signer, grpcKey := newTestECDSAKey(t, "latency-key")
	phone := &testPhoneServer{
		keys:   []*nixkeyv1.SSHKey{grpcKey},
		signer: signer,
	}

	backend, _ := setupTestBackend(t, phone)

	// Start SSH agent with GRPCBackend.
	client, cleanup := startTestAgent(t, backend)
	defer cleanup()

	// Populate key cache.
	keys, err := client.List()
	if err != nil {
		t.Fatalf("list keys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}

	pub, err := ssh.ParsePublicKey(grpcKey.GetPublicKeyBlob())
	if err != nil {
		t.Fatalf("parse public key: %v", err)
	}

	const numRequests = 20
	latencies := make([]time.Duration, numRequests)

	for i := 0; i < numRequests; i++ {
		data := []byte(fmt.Sprintf("latency test payload %d", i))
		start := time.Now()
		sig, err := client.Sign(pub, data)
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("sign request %d: %v", i, err)
		}
		if err := pub.Verify(data, sig); err != nil {
			t.Fatalf("verify request %d: %v", i, err)
		}
		latencies[i] = elapsed
	}

	// Compute p95.
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p95Idx := int(float64(numRequests) * 0.95)
	if p95Idx >= numRequests {
		p95Idx = numRequests - 1
	}
	p95 := latencies[p95Idx]

	t.Logf("Sign latency stats (n=%d):", numRequests)
	t.Logf("  min:    %v", latencies[0])
	t.Logf("  median: %v", latencies[numRequests/2])
	t.Logf("  p95:    %v", p95)
	t.Logf("  max:    %v", latencies[numRequests-1])

	const maxP95 = 2 * time.Second
	if p95 > maxP95 {
		t.Errorf("p95 latency %v exceeds threshold %v", p95, maxP95)
	}
}
