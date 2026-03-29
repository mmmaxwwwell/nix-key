# Phase phase1-test-infrastructure- — Review #1: REVIEW-FIXES

**Date**: 2026-03-29T03:58:00Z
**Fixes applied**:
- `go.mod`: Removed broken `replace github.com/skip2/go-qrcode => /tmp/go-qrcode` directive that pointed to a non-existent directory, preventing compilation of any package importing `internal/pairing`. Commit: baa2399.

**Deferred** (optional improvements, not bugs):
- `go.sum` is missing the checksum entry for `github.com/skip2/go-qrcode` (because the replace directive bypassed the module proxy). Running `go mod tidy` with network access will fix this.
- `internal/pairing/server.go` `handlePair` does not limit request body size with `http.MaxBytesReader` — low risk since the server is temporary and only accessible via Tailscale.
- `internal/pairing/pair.go` `notifyDaemon` builds JSON manually via `fmt.Sprintf` — safe today since `dev.ID` is hex-encoded, but `json.Marshal` would be more robust. Not a current bug.
- `cmd/test-reporter/main.go` has no test files (0% coverage) — acceptable since it's a CLI tool exercised via `make test` pipeline.
