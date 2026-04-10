# Phase phase1-host-go-fixes — Review #1: REVIEW-CLEAN

**Date**: 2026-04-10
**Assessment**: Code is clean. No bugs, security issues, or correctness problems found.

## Code Review: develop

**Scope**: 11 source files changed (excluding vendor), ~770 lines added | **Base**: 4bfb869~1
**Commits**: T009-fix (struct tag validation), T010-fix (shutdown logging), T044-fix (Nix device loading), T054-fix (STATUS column)

### Spec-conformance check

| Task | Status | Notes |
|------|--------|-------|
| T009-fix | Conformant | All 8 validate tags match spec. Both struct tags + custom validate() present. Tests assert "min", "max", "required", "oneof". |
| T010-fix | Conformant | Exactly 5 log messages in correct order. logFlush field called last, nil-safe. All callers updated. Tests verify exact message sequence. |
| T044-fix | Conformant | DeviceConfig JSON tags match Nix module.nix output. *string for optional fields. Contract test verifies round-trip including null values. Integration test verifies merge of both sources. |
| T054-fix | Conformant | Header is exactly "NAME\tTAILSCALE IP\tCERT FINGERPRINT\tLAST SEEN\tSTATUS\tSOURCE". Values are exactly "online"/"offline"/"unknown". Concurrent Ping via WaitGroup+goroutines. SOURCE column preserved. |

### Findings

No issues found. The changes look correct, secure, and well-structured.

### What looks good

- Cross-boundary contract test (T044-fix) is thorough: tests null values, non-null cert paths, and full merge integration.
- STATUS column integration test (T054-fix) sets up a real mTLS gRPC server to verify "online" status, uses unreachable port for "offline", and missing cert paths for "unknown" — all three code paths exercised.
- Shutdown logging (T010-fix) test uses JSON decoder to parse structured log output and verify exact message ordering.

**Deferred** (optional improvements, not bugs):
- `pingDevice` has no dial-level timeout (only the Ping RPC has a 2s context timeout). If TCP SYN to an unreachable Tailscale IP hangs, the goroutine blocks until OS TCP timeout (~2min). Not a spec violation (spec says "2s timeout per device" for the Ping), but could affect UX.
- `d := d` loop variable capture in `probeDeviceStatuses` is unnecessary in Go 1.22+ but harmless.
