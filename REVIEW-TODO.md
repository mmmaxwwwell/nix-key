# Security Code Review — nix-key

**Date**: 2026-03-29
**Reviewer**: T073 automated security review
**Focus**: mTLS correctness, cert pinning, age encryption, no plaintext secrets, gRPC input validation, CI secret leakage

## Summary

| Severity | Count | Fixed |
|----------|-------|-------|
| Critical | 1     | Yes   |
| High     | 3     | Yes   |
| Medium   | 5     | No (documented) |
| Low      | 4     | No (documented) |
| Pass     | 10    | N/A   |

---

## Critical Findings

### C-001: Pairing server uses TLS 1.2 instead of TLS 1.3 [FIXED]

- **File**: `internal/pairing/server.go:110`
- **Issue**: `MinVersion: tls.VersionTLS12` — all other TLS configs in the project enforce TLS 1.3. The pairing server handles sensitive operations (token exchange, cert exchange) and should not allow TLS 1.2 connections.
- **Fix**: Changed to `tls.VersionTLS13`.

---

## High Findings

### H-001: gRPC error messages leak internal details [FIXED]

- **File**: `pkg/phoneserver/server.go:55,113,134`
- **Issue**: Error messages like `status.Errorf(codes.Internal, "list keys: %v", err)` expose the underlying `KeyStore`/`Confirmer` error message to gRPC clients. While the SSH agent sanitizes these before returning to SSH clients (FR-097), a direct gRPC client connected over mTLS would see internal error details.
- **Fix**: Return generic error messages to gRPC clients; log full details server-side.

### H-002: Nil pointer dereference possible in ListKeys/Sign handlers [FIXED]

- **File**: `pkg/phoneserver/server.go:60-66,93-98`
- **Issue**: `kl.Get(i)` can return `nil` if a nil `*Key` was added to the `KeyList`. The server code dereferences the result without a nil check. A buggy or malicious `KeyStore` implementation (Android side) could cause a panic.
- **Fix**: Added nil guard after `kl.Get(i)` with `continue`/skip behavior.

### H-003: CI workflow missing explicit permissions [FIXED]

- **File**: `.github/workflows/ci.yml`
- **Issue**: No `permissions:` block declared. GitHub Actions defaults may grant broader permissions than necessary, increasing risk from malicious fork PRs.
- **Fix**: Added `permissions: { contents: read }` to ci.yml and e2e.yml.

---

## Medium Findings (documented, no fix required)

### M-001: Socket TOCTOU race between remove and listen

- **Files**: `internal/agent/agent.go:95-109`, `internal/daemon/control.go:113-128`
- **Issue**: `os.Remove(socketPath)` then `net.Listen("unix", socketPath)` then `os.Chmod(socketPath, 0600)` creates a small window where the socket exists with default permissions. An attacker in the same user session could theoretically connect during this window.
- **Mitigation**: Parent directory is created with `0700`, so only the owning user can access files within. The TOCTOU window is microseconds. Combined, the practical risk is negligible.
- **Recommendation**: If Go ever supports `net.ListenUnix` with mode flags, adopt it.

### M-002: No umask(0077) set at daemon startup

- **Files**: `cmd/nix-key/daemon.go`
- **Issue**: If the process inherits a permissive umask (e.g., 0022), files could theoretically be created with group/world-readable permissions despite `os.WriteFile(..., 0600)`. However, Go's `os.WriteFile` uses the mode directly (masked by umask), so a permissive umask would weaken the effective permissions.
- **Mitigation**: NixOS systemd service config sets `UMask=0077`. Only affects manual daemon runs.
- **Recommendation**: Add `syscall.Umask(0077)` in daemon `init()` for defense-in-depth.

### M-003: Control socket accepts all commands without per-command auth

- **File**: `internal/daemon/control.go:193-209`
- **Issue**: Any process with the same UID that can connect to the Unix socket can execute all commands including device revocation.
- **Mitigation**: Socket permissions (0600) restrict access to the owning user. This is the standard SSH agent security model.

### M-004: Cert file deletion errors silently ignored on revoke

- **File**: `internal/daemon/control.go:285-303`
- **Issue**: `deleteCertFiles` ignores all `os.Remove` errors. If deletion fails, the encrypted key material remains on disk.
- **Mitigation**: The files are age-encrypted, so exposure risk is limited. The `revoke-device` response still returns "ok".
- **Recommendation**: Log deletion errors for auditability.

### M-005: Integer cast uint32 to int32 for sign flags without bounds check

- **File**: `pkg/phoneserver/server.go:130`
- **Issue**: `int32(req.GetFlags())` could overflow for values > 2^31-1. Practically, SSH agent signature flags are always 0, 2, or 4 (rsa-sha2-256/512).
- **Recommendation**: Add validation that flags are in the valid SSH agent range.

---

## Low Findings (documented only)

### L-001: Test artifacts could contain sensitive paths

- **Files**: `.github/workflows/ci.yml` (artifact upload steps)
- **Issue**: `test-logs/` are uploaded as artifacts and may contain error messages with internal file paths or cert fingerprints.
- **Mitigation**: Test fixtures use deterministic non-real secrets. Test-reporter sanitizes structured output.

### L-002: Gitleaks allowlists entire test/fixtures directory

- **File**: `.gitleaks.toml`
- **Issue**: `test/fixtures/.*` is globally allowlisted. If real secrets were accidentally placed in test/fixtures, gitleaks would not catch them.
- **Mitigation**: CLAUDE.md documents that fixtures are deterministic with fixed seeds. CI also runs Trivy and Semgrep.

### L-003: Missing explicit ALPN in pairing server TLS config

- **File**: `internal/pairing/server.go:107-111`
- **Issue**: Pairing server doesn't set `NextProtos`. This is correct since it's HTTP/1.1, not gRPC/h2. No action needed.

### L-004: Plaintext key material held in Go heap until GC

- **Files**: `internal/pairing/pair.go`, `internal/mtls/age.go`
- **Issue**: After encryption, plaintext key bytes (`clientKeyPEM`, `plaintext` in `EncryptFile`) are not explicitly zeroed from memory.
- **Mitigation**: Go does not provide reliable memory zeroing (GC may have already copied the data). The practical threat model (physical memory dump of the host process) is out of scope for this project. The key material is only held briefly during pairing.

---

## Pass (Verified Secure)

| Area | Status | Notes |
|------|--------|-------|
| mTLS cert pinning | PASS | SHA256 fingerprint pinning with expiry check in `VerifyPeerCertificate` |
| TLS 1.3 minimum (mTLS) | PASS | `MinVersion: tls.VersionTLS13` in `pinning.go:77` |
| `InsecureSkipVerify` usage | PASS | Correctly paired with `VerifyPeerCertificate` for self-signed cert pinning |
| `ClientAuth: RequireAnyClientCert` | PASS | Server requires client cert for mTLS |
| ALPN for gRPC | PASS | `NextProtos: ["h2"]` in `pinning.go:78` |
| crypto/rand throughout | PASS | All key generation uses `crypto/rand.Reader`, never `math/rand` |
| Age encryption at rest | PASS | Private keys encrypted to `.age` files, decrypted only to memory |
| SSH agent error sanitization | PASS | All errors return `errAgentFailure` with no internal details (FR-097) |
| File permissions | PASS | 0600 for secrets, 0700 for secret directories consistently |
| No hardcoded keys in prod | PASS | Test fixtures only in `test/fixtures/`, all runtime keys from crypto/rand |
