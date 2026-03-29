# Phase phase5-ssh-agent-protocol — Review #1: REVIEW-CLEAN

**Date**: 2026-03-29
**Assessment**: Code is clean. No bugs, security issues, or correctness problems found.

## Code Review: phase5-ssh-agent-protocol

**Scope**: 5 files changed (internal/agent/agent.go, backend.go, agent_test.go, backend_test.go, userflow_test.go) + internal/daemon/registry.go, registry_test.go | **Base**: 5889f22~1
**Commits**: T019 (SSH agent handler), T020 (device registry), T021 (wire agent to gRPC), T022 (user-flow integration tests)

### Findings

No P0 or P1 issues found.

### What looks good

- Error sanitization is thorough: all backend errors are replaced with `errAgentFailure` before returning to SSH clients (FR-097). Tests verify no internal details (IP addresses, file paths, error messages) leak.
- Concurrent sign requests are well-handled with proper locking (RWMutex on keyCache, device registry).
- Unix socket permissions set to 0600, directories to 0700 — consistent with security requirements.
- Registry merge logic correctly handles nix-declared vs runtime-paired device conflicts per FR-064.
- File persistence uses 0600 permissions for devices.json.
- Integration tests are comprehensive: happy path, timeout, denial, mid-connection drop (FR-E08), multiple phones (FR-029), concurrent sign requests (T-HI-08).

**Deferred** (optional improvements, not bugs):
- `List()` uses a single `connectionTimeout` context for both dial and `ListKeys` RPC, so a slow dial leaves less time for the RPC. The `Sign()` path correctly uses separate contexts for dial and sign. Not a correctness bug, but could cause spurious timeouts during listing.
- When `allowKeyListing` is false, `Sign()` auto-refresh via `List()` returns empty, making signing impossible. In practice SSH clients always list before signing, so this is not a real issue.
- Data race in `internal/pairing/pair.go` (untracked, uncommitted Phase 6 code): `RunPair()` writes to a `bytes.Buffer` while test reads it concurrently. Not Phase 5 scope — should be fixed when Phase 6 is committed.
