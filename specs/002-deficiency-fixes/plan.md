# Plan: 002-deficiency-fixes

## Overview

Fix 7 deficient task implementations identified by systematic audit. All fixes are scoped to specific files with well-understood requirements. No new architecture — just correcting existing implementations to match their original specs.

## Phase Structure

All 7 fixes are independent but executed sequentially (one task at a time). Grouped into 3 phases for logical ordering.

### Phase 1: Host Go Fixes (T009-fix, T010-fix, T044-fix, T054-fix)

Four Go-side fixes:

1. **T009-fix: Add struct tag validation to Config** — Add `validate:"..."` tags to Config struct fields, add `go-playground/validator/v10` dependency, update `validate()` to run struct tag validation first then custom rules. Update tests to assert specific constraint names in error messages.

2. **T010-fix: Add logging to ShutdownManager** — Inject `*slog.Logger` into `ShutdownManager`, log each shutdown step with exact message strings: `"shutdown initiated"`, `"stopping new connections"`, `"draining in-flight requests"`, `"executing shutdown hooks"`, `"shutdown complete"`. Add log flush callback at end (nil-safe). Update tests to verify all 5 messages in order.

3. **T044-fix: Load Nix devices from config** — Add `Devices map[string]DeviceConfig` to Config struct with `DeviceConfig` fields using JSON tags that exactly match the NixOS module output: `json:"name"`, `json:"tailscaleIp"`, `json:"port"`, `json:"certFingerprint"`, `json:"clientCertPath"`, `json:"clientKeyPath"`. Update `runDaemon()` to pass config devices to `registry.Merge()`. Add cross-boundary test that verifies a config.json written by the NixOS module format round-trips through the Go Config struct.

4. **T054-fix: Add STATUS column to devices CLI** — Add concurrent Ping RPC check (2s timeout) per device. Table header must be exactly: `NAME\tTAILSCALE IP\tCERT FINGERPRINT\tLAST SEEN\tSTATUS\tSOURCE`. STATUS values are exactly `"online"`, `"offline"`, `"unknown"`. Keep existing SOURCE column.

### Phase 2: Script Fix (T072-fix)

5. **T072-fix: Fix verify-release-pipeline.sh grep patterns** — Section 4 must grep for `workflow_run:` (not `push:`) and `workflow_call:` (not `pull_request:`) to match e2e.yml's actual triggers.

### Phase 3: Android Fixes (T034-fix, T089-fix)

6. **T034-fix: Implement real TailscaleBackend** — Create `pkg/tsbridge/tailscale.go` wrapping `tsnet.Server`. Go `Start(authKey, dataDir string) (string, error)` maps to Kotlin's existing `fun start(authKey: String?, dataDir: String): String?` via gomobile's `(string, error)` → `String?` + exception bridge. Create `RealTailscaleBackend.kt`, replace stub in `AppModule.kt`. Keep stub as `FakeTailscaleBackend.kt` in `androidTest/`.

7. **T089-fix: Add loading states to exactly 3 screens** — `ServerListScreen`, `KeyListScreen`, `KeyDetailScreen`. Each gets `isLoading: StateFlow<Boolean>` in its ViewModel + `CircularProgressIndicator` in its Composable. (The other screens — `PairingScreen`, `TailscaleAuthScreen` — already have loading states and are NOT in scope.)

## Interface Contracts

| IC | Name | Producer | Consumer | Wire Format (exact JSON keys) |
|----|------|----------|----------|-------------------------------|
| IC-D01 | Nix device config | NixOS module (`nix/module.nix`) | Go Config struct (`internal/config/config.go`) | `{"devices": {"<id>": {"name": string, "tailscaleIp": string, "port": int, "certFingerprint": string, "clientCertPath": string\|null, "clientKeyPath": string\|null}}}` |
| IC-D02 | Devices CLI output | `cmd/nix-key/devices.go` | User / scripts | Tab-separated columns: `NAME`, `TAILSCALE IP`, `CERT FINGERPRINT`, `LAST SEEN`, `STATUS`, `SOURCE` |
| IC-D03 | Shutdown log messages | `internal/daemon/shutdown.go` | `journalctl` / log consumers | JSON log lines with `msg` field containing exact strings: `"shutdown initiated"`, `"stopping new connections"`, `"draining in-flight requests"`, `"executing shutdown hooks"`, `"shutdown complete"` |
| IC-D04 | TailscaleService gomobile bridge | `pkg/tsbridge/tailscale.go` | `RealTailscaleBackend.kt` | Go `Start(string, string) (string, error)` → Kotlin `fun start(String?, String): String?` + exception. Go `GetIP() (string, error)` → Kotlin `fun getIp(): String?` + exception. |

## Testing Strategy

Each fix adds or updates tests. Contract tests verify cross-boundary format agreement.

| Fix | Test Type | What to verify | Exact assertions |
|-----|-----------|----------------|------------------|
| T009-fix | Unit | Struct tag validation catches invalid fields | Error contains `"min"`, `"max"`, `"required"`, `"oneof"` per field |
| T010-fix | Unit | Shutdown logs appear in order | Buffer contains exactly 5 INFO messages in sequence; flush callback invoked last |
| T044-fix | Unit + contract | Config loads devices; Nix→Go round-trip | JSON with keys `name`, `tailscaleIp`, `port`, `certFingerprint`, `clientCertPath`, `clientKeyPath` deserializes correctly; null values handled |
| T054-fix | Integration | STATUS column shows online/offline | Output contains `STATUS` column header (not `SOURCE` alone); reachable device → `"online"`, unreachable → `"offline"` |
| T072-fix | Script | Script exits 0 | `bash scripts/verify-release-pipeline.sh` returns 0 |
| T034-fix | Unit + instrumented | TailscaleService wraps tsnet; FakeTailscaleBackend tests interface | Go `TailscaleService` compiles with real `tsnet` import; Kotlin tests pass with fake |
| T089-fix | UI test | Loading indicators on exactly 3 screens | `CircularProgressIndicator` displayed in `ServerListScreen`, `KeyListScreen`, `KeyDetailScreen` |

## Success Criteria Mapping

| SC | Fix | Test Tier | Assertion |
|----|-----|-----------|-----------|
| SC-100 | T044-fix | Integration | Devices from both Nix config and devices.json visible in registry |
| SC-101 | T054-fix | Integration | Output contains column headed exactly `STATUS` with values `online`/`offline`/`unknown` |
| SC-102 | T009-fix | Unit | Error messages contain constraint names (`min`, `required`, `oneof`) |
| SC-103 | T010-fix | Unit | Log buffer contains `"shutdown initiated"` as first message |
| SC-104 | T089-fix | UI | `CircularProgressIndicator` present in all 3 enumerated screens |
| SC-105 | T072-fix | Script | Exit code 0 |
| SC-106 | T034-fix | Manual/smoke | Real Tailnet IP assigned (not `"100.100.100.100"`) |
| SC-107 | All | Regression | `make validate` + `nix flake check` + `./gradlew testDebugUnitTest` all pass |
