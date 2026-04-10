# Feature Specification: Implementation Deficiency Fixes

**Feature Branch**: `develop` (direct commits, no feature branch)  
**Created**: 2026-04-09  
**Status**: Draft  
**Input**: Audit of all completed tasks (T001-T098, T100-T113) found 7 deficient implementations

## Context

A systematic audit of all 98+ completed tasks against their original specifications found 7 tasks where the implementation does not satisfy the requirements. This feature re-implements the deficient portions without changing any working functionality.

## User Scenarios & Testing

### User Story 1 - Nix-declared device merge works at runtime (Priority: P1)

The NixOS module declares devices in `services.nix-key.devices`, and the daemon merges them with runtime-paired devices from `devices.json`. Currently the daemon never reads Nix-declared devices — it passes `nil` to `registry.Merge()`.

**Why this priority**: This is a core architectural feature — without it, Nix-declared devices are silently ignored, breaking the declarative device management story entirely.

**Independent Test**: Configure a NixOS module with a device, start the daemon, verify `nix-key devices` lists both Nix-declared and runtime-paired devices. Verify Nix-declared devices cannot be revoked via CLI.

**Acceptance Scenarios**:

1. **Given** a NixOS config with `services.nix-key.devices.myphone = {...}`, **When** the daemon starts, **Then** the device appears in `nix-key devices` output with source "nix-declared"
2. **Given** both a Nix-declared device and a runtime-paired device, **When** daemon starts, **Then** both are visible and keys from both are available for signing
3. **Given** a Nix-declared device, **When** `nix-key revoke myphone` is attempted, **Then** the command fails with an error directing the user to remove the device from Nix config

---

### User Story 2 - Device connectivity status shown in CLI (Priority: P2)

`nix-key devices` shows a STATUS column indicating whether each device is currently reachable (online/offline), not just its provenance source.

**Why this priority**: Users need to quickly see which devices are available for signing. SOURCE is useful but doesn't answer "can I sign right now?".

**Independent Test**: Start daemon with one reachable and one unreachable device, run `nix-key devices`, verify STATUS column shows "online" and "offline" respectively.

**Acceptance Scenarios**:

1. **Given** a paired device that is reachable via Tailscale, **When** `nix-key devices` is run, **Then** the STATUS column shows "online"
2. **Given** a paired device whose phone app is closed, **When** `nix-key devices` is run, **Then** the STATUS column shows "offline"
3. **Given** a device that has never been seen, **When** `nix-key devices` is run, **Then** the STATUS column shows "unknown"

---

### User Story 3 - Config validation uses struct tags (Priority: P2)

The Go config module validates fields using Go struct tags (e.g., `validate:"required,min=1,max=65535"`) in addition to the existing custom validation, as originally specified in T009.

**Why this priority**: Struct tag validation is declarative, self-documenting, and catches errors that custom validation might miss as new fields are added.

**Independent Test**: Add an invalid config value for a tagged field, verify the error message references the struct tag constraint.

**Acceptance Scenarios**:

1. **Given** a config with `port: 0`, **When** config loads, **Then** validation fails with a message about min value constraint
2. **Given** a config with `port: 70000`, **When** config loads, **Then** validation fails with a message about max value constraint
3. **Given** a config with an empty `tailscaleInterface`, **When** config loads, **Then** validation fails with a "required" constraint error
4. **Given** a config with `logLevel: "trace"`, **When** config loads, **Then** validation fails with an "oneof" constraint error

---

### User Story 4 - Shutdown sequence logs initiation and flushes logs (Priority: P3)

The `ShutdownManager` logs "shutdown initiated" at the start and flushes the logger at the end, ensuring all log output is captured before process exit.

**Why this priority**: Missing shutdown logs make it impossible to diagnose shutdown issues in production. Log flushing prevents the last few log lines from being lost.

**Independent Test**: Send SIGTERM to daemon, verify "shutdown initiated" appears in logs, verify all subsequent shutdown steps are logged.

**Acceptance Scenarios**:

1. **Given** a running daemon, **When** SIGTERM is received, **Then** "shutdown initiated" is logged at INFO level before any hooks run
2. **Given** a running daemon with registered hooks, **When** shutdown completes, **Then** the logger is flushed (sync) after all hooks have run
3. **Given** a running daemon, **When** shutdown completes, **Then** the sequence of log messages shows: initiated -> stop accepting -> drain in-flight -> hooks (reverse order) -> flush

---

### User Story 5 - Android screens show loading states (Priority: P3)

`ServerListScreen`, `KeyListScreen`, and `KeyDetailScreen` show loading indicators (spinner/progress) during initial data fetch, matching the pattern already used by `PairingScreen` and `TailscaleAuthScreen`.

**Why this priority**: Without loading states, screens flash empty/stale content before data arrives. This is a polish issue but affects perceived quality.

**Independent Test**: Open each screen, verify a loading indicator appears briefly before data is displayed.

**Acceptance Scenarios**:

1. **Given** the app is launched, **When** `ServerListScreen` is displayed, **Then** a `CircularProgressIndicator` shows while hosts are being loaded
2. **Given** the user navigates to keys, **When** `KeyListScreen` is displayed, **Then** a loading indicator shows while keys are fetched from KeyManager
3. **Given** the user taps a key, **When** `KeyDetailScreen` is displayed, **Then** a loading indicator shows while key details are fetched

---

### User Story 6 - Release pipeline verification script is correct (Priority: P3)

`scripts/verify-release-pipeline.sh` grep patterns match the actual workflow YAML files, so the script validates correctly instead of false-failing.

**Why this priority**: A verification script that always fails is worse than no script — it trains people to ignore failures.

**Independent Test**: Run `scripts/verify-release-pipeline.sh` and verify it exits 0 against the current workflow files.

**Acceptance Scenarios**:

1. **Given** the current `.github/workflows/e2e.yml` using `workflow_run:` trigger, **When** the script checks E2E triggers, **Then** the check passes (greps for `workflow_run:` instead of `push:`)
2. **Given** the current workflow files, **When** the full script runs, **Then** all checks pass and exit code is 0

---

### User Story 7 - TailscaleBackend has real tsnet integration (Priority: P3)

The Android `TailscaleBackend` implementation wraps real `tsnet.Server` (Go Tailscale library) via gomobile instead of returning a hardcoded stub IP. This is the final piece needed for the app to actually connect to Tailscale.

**Why this priority**: Without this, the Android app cannot connect to any Tailnet. However, this is P3 because (a) E2E tests use headscale with the phonesim (Go process, not Android app), and (b) implementing this requires the gomobile AAR which has its own build complexity.

**Independent Test**: Build the APK with the real backend, install on an emulator with network access, verify Tailscale auth flow completes and a real Tailscale IP is assigned.

**Acceptance Scenarios**:

1. **Given** a valid Tailscale auth key, **When** `TailscaleBackend.start()` is called, **Then** it returns null (auth completed), the device joins the Tailnet, and `getIp()` returns a real 100.x.y.z Tailscale IP
2. **Given** a running Tailscale session, **When** `stop()` is called, **Then** the device leaves the Tailnet and `isRunning()` returns false
3. **Given** an invalid auth key, **When** `start()` is called, **Then** the gomobile bridge throws an exception indicating auth failure

---

### Edge Cases

- What happens if struct tag validation and custom validation conflict? Custom validation takes precedence as a second pass.
- What happens if Nix config.json has a `devices` field but it's malformed? Daemon logs error and falls back to runtime-only devices.
- What happens if connectivity check times out during `nix-key devices`? Show "offline" in STATUS column (timeout is a subcase of offline), don't block the whole table.
- What happens if the logger is already closed when flush is called? No-op, no panic.

## Requirements

### Functional Requirements

- **FR-300**: Go Config struct MUST include validation struct tags (e.g., `validate:"required,min=1,max=65535"`) on all validated fields, using `go-playground/validator` or equivalent
- **FR-301**: Config `validate()` MUST run struct tag validation first, then apply custom validation rules as a second pass
- **FR-302**: `ShutdownManager` MUST accept a logger and log "shutdown initiated" at INFO level before executing any shutdown hooks
- **FR-303**: `ShutdownManager` MUST flush (sync) the logger after all hooks have completed, as the final step before returning
- **FR-304**: `ShutdownManager` MUST log each shutdown step: "stopping new connections", "draining in-flight requests", "executing shutdown hooks", "shutdown complete"
- **FR-305**: Go Config struct MUST include a `Devices` field (`map[string]DeviceConfig`) that is populated from `config.json`
- **FR-306**: `runDaemon()` MUST read Nix-declared devices from the loaded config and pass them to `registry.Merge(nixDevices, runtimeDevices)`
- **FR-307**: `DeviceConfig` struct MUST include fields with JSON tags matching the NixOS module output: `Name` (`json:"name"`), `TailscaleIP` (`json:"tailscaleIp"`), `Port` (`json:"port"`), `CertFingerprint` (`json:"certFingerprint"`), `ClientCertPath` (`json:"clientCertPath"`, optional/nullable), `ClientKeyPath` (`json:"clientKeyPath"`, optional/nullable)
- **FR-308**: `nix-key devices` MUST show a STATUS column with connectivity state: "online" (Ping RPC succeeds), "offline" (Ping fails, times out, or any connection error), "unknown" (no cert info available to attempt connection). There are exactly three STATUS values — timeout is a subcase of "offline"
- **FR-309**: STATUS check MUST use a short timeout (2s) per device and run checks concurrently to avoid blocking
- **FR-310**: `nix-key devices` MUST still show the SOURCE column alongside STATUS (both columns present)
- **FR-311**: `scripts/verify-release-pipeline.sh` MUST use correct grep patterns matching actual workflow YAML (e.g., `workflow_run:` for e2e.yml, not `push:`)
- **FR-312**: `ServerListScreen`, `KeyListScreen`, and `KeyDetailScreen` MUST show a `CircularProgressIndicator` while data is loading
- **FR-313**: Each ViewModel for the above screens MUST expose an `isLoading` state that is `true` during initial data fetch
- **FR-314**: Loading indicator MUST be replaced by content once data arrives, or by an error state if fetch fails
- **FR-315**: Android `TailscaleBackend` MUST wrap `tsnet.Server` (Go Tailscale library) via gomobile — the same library used by phonesim
- **FR-316**: `TailscaleBackend.start()` MUST initialize a `tsnet.Server`, authenticate with the provided auth key, and return an OAuth URL (if interactive auth needed) or null (if auth key succeeded). On failure, the gomobile bridge throws an exception to the Kotlin caller. This matches the existing `TailscaleBackend` Kotlin interface: `fun start(authKey: String?, dataDir: String): String?`
- **FR-317**: `TailscaleBackend.getIp()` MUST return the actual Tailscale IP assigned to the device, not a hardcoded value
- **FR-318**: `TailscaleBackend.stop()` MUST cleanly shut down the Tailscale connection

### Key Entities

- **DeviceConfig**: New struct in `internal/config/` representing a Nix-declared device (name, tailscaleIp, port, certFingerprint, clientCertPath, clientKeyPath)
- **TailscaleBackend**: Existing interface in Android DI, currently stubbed — needs real implementation wrapping `tsnet.Server` via gomobile

## Success Criteria

### Measurable Outcomes

- **SC-100**: `nix-key devices` shows both Nix-declared and runtime-paired devices with correct SOURCE labels
- **SC-101**: `nix-key devices` shows STATUS column with accurate online/offline state
- **SC-102**: Config validation errors reference struct tag constraints (not just custom messages)
- **SC-103**: Daemon shutdown logs are visible in `journalctl` output
- **SC-104**: All three Android list/detail screens show loading indicators during data fetch
- **SC-105**: `scripts/verify-release-pipeline.sh` exits 0 against current workflow files
- **SC-106**: Android app connects to a real Tailnet with a valid auth key (manual/smoke test — real Tailscale auth keys are not available in CI; the Go `tsnet` path is validated by the existing headscale+phonesim E2E tests)
- **SC-107**: All existing tests continue to pass (no regressions)

## Assumptions

- The existing `registry.Merge()` function correctly handles two-source merge — only the calling code needs to change
- `go-playground/validator` (or equivalent) is acceptable as a new dependency for struct tag validation
- tsnet via gomobile AAR is available (the gomobile build pipeline already works per T110-T111)
- The NixOS module already writes `devices` to `config.json` correctly — only the Go daemon's reading side is broken

## Non-Goals

- Rewriting any working functionality — only the 7 deficient areas are in scope
- Adding new features beyond what was originally specified in the tasks
- Changing the NixOS module's device configuration format
- Adding loading states to screens that don't perform async operations
