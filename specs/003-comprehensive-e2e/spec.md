# Feature Specification: Comprehensive E2E Testing (Clean Slate)

**Feature Branch**: `develop` (direct commits)
**Created**: 2026-04-09
**Status**: Draft
**Input**: Complete re-exploration of the app after 002-deficiency-fixes, plus coverage of all functions 001-e2e never tested

## Context

Feature 001-e2e-android-testing explored the Android app via MCP, but against a partially broken app (stub TailscaleBackend, missing loading states, broken Nix device merge). Feature 002-deficiency-fixes corrects 7 implementations. This feature does a clean-slate re-exploration of the entire app plus every function 001 never covered, and generates deterministic regression tests inline — not as a deferred batch step.

## User Scenarios & Testing

### User Story 1 — Full screen exploration with inline regression tests (Priority: P1)

Agent explores every screen on the live emulator via MCP tools, comparing against UI_FLOW.md. For each bug found and fixed, the agent writes a UI Automator regression test BEFORE marking the task done. Every fix gets a test — no deferred batching.

**Why this priority**: This is the core product: verify every screen works, find visual bugs, fix them, and protect the fixes with CI tests.

**Screens to explore** (one task per screen, exactly matching UI_FLOW.md):
1. `TailscaleAuthScreen` — auth key field, connect button, OAuth button, connection indicator, validation errors
2. `ServerListScreen` — paired hosts list, connection status dots, Tailnet indicator, empty state, pull-to-refresh, gear icon, loading state (new from 002)
3. `PairingScreen` — deep link + QR entry, pairing phases, cancel/done, error messages, OTEL prompt
4. `KeyManagementScreen` — key list, FAB create, lock/unlock indicators, long-press re-lock context menu, loading state (new from 002)
5. `KeyDetailScreen` — create mode (both Ed25519 + ECDSA-P256), view/edit mode, name editing, unlock policy picker, signing policy picker, lock/unlock button, export (clipboard/share/QR), delete, security warning dialogs, loading state (new from 002)
6. `SettingsScreen` — key listing toggle, default policy pickers, tracing toggle + OTEL endpoint, Tailscale section, about section
7. `SignRequestDialog` — approve/deny, biometric simulation, dialog dismiss, host name + key name + data hash display

**Acceptance Scenarios**:
1. **Given** each screen, **When** the agent explores it via MCP, **Then** findings.json records pass/fail for every element and interaction
2. **Given** a bug found and fixed, **When** the agent marks the task done, **Then** a UI Automator regression test exists for that bug in `android/app/src/androidTest/java/com/nixkey/e2e/regression/`
3. **Given** all screen tasks complete, **When** `./gradlew connectedDebugAndroidTest` runs, **Then** all regression tests pass

---

### User Story 2 — Navigation flow validation (Priority: P1)

Agent exercises every navigation edge from the UI_FLOW.md flowchart on the live emulator, including back navigation and back-stack clearing.

**Acceptance Scenarios**:
1. **Given** first launch, **When** auth completes, **Then** ServerList is shown and back button exits app (back stack cleared)
2. **Given** ServerList, **When** agent follows every outgoing edge (Pairing, KeyManagement, Settings) and returns, **Then** navigation is correct at every step

---

### User Story 3 — Cross-system sign round-trip (Priority: P1)

Verify the full signing path: host `ssh-keygen -Y sign` → SSH_AUTH_SOCK → daemon → gRPC over mTLS over headscale → phone → user approval → signature returned → SSH succeeds. Test approve, deny, and timeout paths.

**Acceptance Scenarios**:
1. **Given** a paired setup, **When** sign request triggered and approved, **Then** SSH operation exits 0
2. **Given** a sign request, **When** denied, **Then** SSH fails with SSH_AGENT_FAILURE
3. **Given** a sign request with no action, **When** signTimeout elapses, **Then** SSH fails with SSH_AGENT_FAILURE
4. **Given** 3 rapid sign requests, **When** approved in order, **Then** all 3 succeed

---

### User Story 4 — Multi-key and ECDSA-P256 (Priority: P2)

Create both Ed25519 and ECDSA-P256 keys, verify both listed by host, sign with each.

**Acceptance Scenarios**:
1. **Given** both key types created, **When** `ssh-add -L` runs on host, **Then** both keys listed
2. **Given** a P256 key, **When** sign request triggered for it, **Then** signature succeeds

---

### User Story 5 — Device revocation end-to-end (Priority: P2)

Revoke a device, verify the phone can no longer sign (mTLS rejected).

**Acceptance Scenarios**:
1. **Given** a revoked device, **When** sign request triggered, **Then** SSH fails with SSH_AGENT_FAILURE
2. **Given** revocation, **When** `nix-key devices` runs, **Then** the device is gone

---

### User Story 6 — Key export, security warnings, lock/unlock (Priority: P2)

MCP exploration of functions 001 never tested.

**Acceptance Scenarios**:
1. **Given** key detail screen, **When** "Copy to Clipboard" tapped, **Then** snackbar confirmation appears
2. **Given** auto-approve selected, **When** policy changed, **Then** security warning dialog with exact UI_FLOW.md text appears
3. **Given** an unlocked key, **When** long-pressed on KeyManagement, **Then** "Re-lock key" context menu appears

---

### User Story 7 — Multiple hosts and connection state transitions (Priority: P2)

Pair with 2 hosts, verify both visible. Test Tailnet indicator transitions.

**Acceptance Scenarios**:
1. **Given** 2 paired hosts, **When** ServerList shown, **Then** both appear with names, IPs, status dots
2. **Given** connected state, **When** tailscaled killed, **Then** indicator goes red; restart → green

---

### User Story 8 — CLI exercise (Priority: P2)

Exercise every `nix-key` CLI subcommand against the paired setup.

**Acceptance Scenarios**:
1. **Given** a paired setup, **When** `nix-key status` runs, **Then** output contains `"running"` and device count
2. **Given** a paired setup, **When** `nix-key devices` runs, **Then** output contains `"STATUS"` and `"SOURCE"` column headers
3. **Given** a key, **When** `nix-key export <fingerprint>` runs, **Then** output starts with `ssh-ed25519` or `ecdsa-sha2-nistp256`
4. **Given** a reachable device, **When** `nix-key test <device>` runs, **Then** output contains `"success"`

---

### User Story 9 — OTEL distributed tracing (Priority: P3)

Verify trace propagation across host↔phone with Jaeger collector.

**Acceptance Scenarios**:
1. **Given** OTEL endpoint in pairing QR, **When** sign request completed, **Then** Jaeger API returns trace with spans from both services
2. **Given** the trace, **When** phone spans inspected, **Then** they are children of host spans via traceparent

---

### User Story 10 — Network resilience (Priority: P3)

Test daemon restart, app restart, headscale restart.

**Acceptance Scenarios**:
1. **Given** daemon restarted, **When** sign request triggered, **Then** succeeds (registry loaded from disk)
2. **Given** app force-stopped and restarted, **When** sign request triggered, **Then** succeeds after gRPC re-bind
3. **Given** headscale restarted (30s downtime), **When** signing attempted after reconnect, **Then** succeeds

---

### User Story 11 — Error paths and persistence (Priority: P2)

Re-validate all field validation error messages from UI_FLOW.md and state persistence across force-stop.

**Acceptance Scenarios**:
1. **Given** invalid auth key, **When** entered, **Then** exact error message "Invalid auth key format"
2. **Given** state (host + key), **When** app force-stopped and restarted, **Then** host in ServerList, key in KeyManagement, key shows locked

---

### Edge Cases

- Concurrent sign while key is locked → first triggers unlock, rest queue behind
- Key creation during sign request → sign dialog stays visible
- Revocation of Nix-declared device → error directing to NixOS config
- Empty OTEL endpoint in QR → pairing succeeds, tracing disabled
- P256 key on emulator without StrongBox → software fallback works

## Requirements

### Functional Requirements

#### Infrastructure (Phase 1)
- **FR-500**: Emulator boots, APK installs, MCP tools respond (Screenshot, DumpHierarchy, Click, Type)
- **FR-501**: Deep link pairing bypass works (`nix-key://pair?payload=...`)
- **FR-502**: Fingerprint simulation works (`adb -e emu finger touch 1`)
- **FR-503**: Headscale mesh running with daemon + emulator app
- **FR-504**: Second host pre-auth key and Jaeger collector available in setup.sh

#### MCP Exploration (Phase 2-3)
- **FR-510**: Every screen from UI_FLOW.md explored with MCP on the live emulator
- **FR-511**: Every navigation edge from UI_FLOW.md flowchart exercised
- **FR-512**: Loading states verified on `ServerListScreen`, `KeyListScreen`, `KeyDetailScreen` (CircularProgressIndicator visible during fetch)
- **FR-513**: For each bug found and fixed, a UI Automator regression test MUST be written in the SAME task — not deferred
- **FR-514**: Regression tests MUST use `NixKeyE2EHelper` methods and be placed in `android/app/src/androidTest/java/com/nixkey/e2e/regression/`

#### Extended Exploration (Phase 4)
- **FR-520**: Key export (clipboard, share, QR) verified on KeyDetailScreen
- **FR-521**: Security warning dialogs (auto-approve, none-unlock) verified with exact text from UI_FLOW.md
- **FR-522**: Connection state transitions (green→red→green) verified on Tailnet indicator
- **FR-523**: Long-press re-lock + lock/unlock button verified
- **FR-524**: Multiple paired hosts visible on ServerList
- **FR-525**: ECDSA-P256 key creation path verified (info text, type badge)

#### Cross-System (Phase 5)
- **FR-530**: Sign approve round-trip: SSH exits 0
- **FR-531**: Sign deny round-trip: SSH fails with SSH_AGENT_FAILURE
- **FR-532**: Sign timeout: no user action → SSH_AGENT_FAILURE after signTimeout
- **FR-533**: Concurrent signs: 3 rapid requests, all approved in order, all succeed
- **FR-534**: Multi-key: Ed25519 + ECDSA-P256 both listed and signable
- **FR-535**: Device revocation: `nix-key revoke` → subsequent sign fails

#### CLI (Phase 5)
- **FR-540**: `nix-key status` shows running state, device count
- **FR-541**: `nix-key devices` shows STATUS and SOURCE columns
- **FR-542**: `nix-key export` outputs SSH public key format
- **FR-543**: `nix-key test` reports success with latency

#### Error Paths + Persistence (Phase 3)
- **FR-550**: All field validation error messages match UI_FLOW.md Field Validation Reference Table exactly
- **FR-551**: State (hosts, keys) survives force-stop and restart
- **FR-552**: Keys show locked after restart (unlock state resets)

#### Resilience + OTEL (Phase 6)
- **FR-560**: Daemon restart → signing resumes
- **FR-561**: App force-stop + restart → signing resumes after re-bind
- **FR-562**: Headscale restart → Tailscale reconnects, signing resumes
- **FR-563**: OTEL trace with host + phone spans, parent-child via traceparent

## Success Criteria

- **SC-300**: All 7 screens explored, findings.json has entries for every screen
- **SC-301**: All navigation edges exercised
- **SC-302**: Every bug fixed has a regression test — zero unprotected fixes
- **SC-303**: Sign approve, deny, timeout, concurrent all work
- **SC-304**: Multi-key (Ed25519 + P256) signing works
- **SC-305**: Device revocation prevents signing
- **SC-306**: All CLI commands produce correct output
- **SC-307**: Error messages match spec exactly
- **SC-308**: State persists across restart
- **SC-309**: OTEL distributed trace verified
- **SC-310**: Network resilience (3 restart scenarios) all pass
- **SC-311**: `./gradlew connectedDebugAndroidTest` passes with all regression tests
- **SC-312**: All existing tests pass (no regressions)

## Assumptions

- 002-deficiency-fixes is complete
- Android emulator boots and real APK installs
- gomobile AAR is real (not stub) — TailscaleService from `pkg/tsbridge/`
- `nix-mcp-debugkit` available as flake input
- Headscale + tailscale infrastructure works

## Non-Goals

- Physical device testing
- Real Tailscale control plane (headscale only)
- Performance benchmarking
- iOS/desktop testing
- NixOS VM test rewrite
