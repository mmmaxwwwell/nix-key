# Feature Specification: Comprehensive E2E Integration Testing for nix-key Android App

**Feature Branch**: `001-e2e-android-testing`  
**Created**: 2026-04-06  
**Status**: Draft  
**Input**: User description: "Comprehensive E2E Integration Testing for nix-key Android App"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Agent-driven screen and flow validation on live emulator (Priority: P1)

An agent boots the Android emulator, installs the debug APK, and uses MCP tools (Screenshot, DumpHierarchy, Click, Type, WaitForElement) to walk every screen and navigation flow defined in UI_FLOW.md. The agent compares what it sees on the live screen against the spec and reports discrepancies as bugs in findings.json.

**Why this priority**: This is the core capability. Without agents actually interacting with the live app, the feature has zero value. Every other story depends on this working.

**Independent Test**: Boot emulator, install APK, give agent MCP tools, point it at UI_FLOW.md. Agent navigates every screen, verifies elements exist, and produces findings.json with pass/fail per screen.

**Acceptance Scenarios**:

1. **Given** a clean Android emulator (API 34, x86_64) with the debug APK installed and MCP-android connected, **When** an agent explores the TailscaleAuth screen, **Then** it takes a screenshot, dumps the view hierarchy, and verifies the auth key field, connect button, and loading states match UI_FLOW.md.
2. **Given** the agent has verified all 7 screens individually, **When** it exercises the first-launch navigation flow (TailscaleAuth → enter pre-auth key → ServerList), **Then** it verifies the transition works and the back stack is cleared (pressing Back exits the app).
3. **Given** the agent has completed exploration, **When** findings.json is written, **Then** every screen and flow from UI_FLOW.md has a pass/fail entry with screenshot evidence.

---

### User Story 2 - Cross-system sign request round-trip (Priority: P1)

The agent sets up the full infrastructure (headscale mesh, host tailscale, nix-key daemon, emulator with app) and exercises the core use case: host triggers an SSH sign request, the agent sees the sign dialog appear on the emulator via MCP, approves it, and verifies the signature is returned to the host.

**Why this priority**: This is the product's raison d'etre — SSH signing via phone. If this doesn't work end-to-end, nothing else matters.

**Independent Test**: Start headscale + tailscale + daemon + emulator with paired app. Trigger `ssh-keygen -Y sign` from host. Agent sees dialog via Screenshot, taps Approve via Click, verifies SSH operation completes.

**Acceptance Scenarios**:

1. **Given** headscale mesh is running with nix-key daemon and emulator app paired, **When** the host triggers a sign request, **Then** the sign request dialog appears on the emulator within 5 seconds (verified via MCP Screenshot + WaitForElement).
2. **Given** a sign request dialog is showing, **When** the agent taps Approve and biometric auth succeeds (emulator fingerprint simulation), **Then** the signature is returned and the SSH operation exits 0.
3. **Given** a sign request dialog is showing, **When** the agent taps Deny, **Then** SSH_AGENT_FAILURE is returned and the SSH operation fails.

---

### User Story 3 - Error path validation on live emulator (Priority: P2)

The agent exercises every error path from the Field Validation Reference Table in UI_FLOW.md: invalid auth keys, malformed QR payloads, duplicate key names, invalid OTEL endpoints. For each error, the agent injects the invalid input via MCP tools, takes a screenshot, and verifies the exact error message matches the spec.

**Why this priority**: Error handling is where security tools fail silently. The existing tests never exercise error paths.

**Independent Test**: Agent uses MCP Type/SetText to enter invalid inputs on each screen, verifies error messages appear as specified.

**Acceptance Scenarios**:

1. **Given** the TailscaleAuth screen is shown, **When** the agent enters "not-a-real-key" via MCP SetText, **Then** the validation error "Invalid auth key format" is visible in the view hierarchy.
2. **Given** the KeyDetail create screen is shown, **When** the agent enters a name longer than 64 characters, **Then** the error "Name must be 1-64 characters..." is displayed.
3. **Given** the Settings screen is shown, **When** the agent enters "invalid:endpoint:format" in the OTEL field, **Then** the error "Invalid endpoint format (expected host:port)" appears.

---

### User Story 4 - Persistence and recovery on live emulator (Priority: P2)

The agent creates app state (pairs a host, creates a key), then kills the app process via adb, restarts it, and verifies state integrity via MCP tools (DumpHierarchy to check elements are still present).

**Why this priority**: Persistence bugs cause data loss and are dangerous for a security tool.

**Independent Test**: Agent creates state, force-stops app, restarts, verifies via MCP DumpHierarchy.

**Acceptance Scenarios**:

1. **Given** a host is paired and a key exists, **When** the app is force-stopped and restarted, **Then** the agent verifies the host still appears in ServerList and the key still appears in KeyManagement via DumpHierarchy.
2. **Given** a key is unlocked, **When** the app is stopped and restarted, **Then** the key shows locked state (unlock resets per spec).

---

### User Story 5 - Explore-fix-verify loop (Priority: P3)

When the exploration agent discovers bugs (missing UI elements, wrong error messages, broken navigation), a fix agent receives the findings, applies source code fixes, the runner rebuilds and reinstalls the APK, and a verify agent re-checks each bug on the live emulator. This loops until all bugs are resolved or max iterations reached.

**Why this priority**: The explore-fix-verify loop is what makes this actionable rather than just a reporter. But it depends on all exploration working first.

**Independent Test**: Intentionally introduce a known bug (e.g., wrong error message text), run the loop, verify detection → fix → rebuild → verify completes in one cycle.

**Acceptance Scenarios**:

1. **Given** the explore agent found 3 bugs in findings.json, **When** the fix agent runs, **Then** it modifies source code to fix all 3 bugs and commits changes.
2. **Given** fixes are committed and APK is rebuilt+reinstalled, **When** the verify agent re-checks each bug via MCP tools, **Then** each bug status updates to "fixed" in findings.json.
3. **Given** a fix introduces a regression, **When** the verify agent discovers the regression, **Then** the regression is added to findings.json as a new bug for the next cycle.

---

### Edge Cases

- What happens when the emulator fails to boot within the timeout (120s with KVM)?
- How does the system handle MCP-android losing connection to the emulator mid-test?
- What happens when multiple sign requests arrive simultaneously?
- What happens when the app crashes during exploration (agent detects via Screenshot showing home screen)?

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The runner MUST boot an Android emulator and verify it is responsive before giving agents MCP tools
- **FR-002**: The runner MUST build the debug APK and install it on the emulator before each test run
- **FR-003**: The runner MUST provide MCP-android tools (Screenshot, DumpHierarchy, Click, ClickBySelector, Swipe, Type, SetText, Press, WaitForElement, GetScreenInfo) to agents via --mcp-config
- **FR-004**: Agents MUST visit all 7 screens from UI_FLOW.md (TailscaleAuth, ServerList, Pairing, KeyManagement, KeyDetail, SignRequestDialog, Settings) and verify layout elements match the spec
- **FR-005**: Agents MUST exercise every navigation flow from the UI_FLOW.md flowchart end-to-end on the live emulator
- **FR-006**: The test infrastructure MUST set up a real headscale mesh with pre-authorized auth keys for Tailscale authentication (no mocks)
- **FR-007**: Agents MUST use test bypass mechanisms for hardware-dependent features: deep link for QR scanning (`nix-key://pair?payload=<base64>`), emulator fingerprint simulation for biometrics, software keystore fallback for hardware keystore
- **FR-008**: Agents MUST verify every error message from the Field Validation Reference Table matches the spec text exactly
- **FR-009**: The explore-fix-verify loop MUST support fixing bugs across any codebase area (Android, Go, Nix, proto)
- **FR-010**: All bug findings MUST be recorded in structured findings.json format with screenshots as evidence
- **FR-011**: Persistence tests MUST use adb force-stop/restart (not just activity recreation) to verify data survival
- **FR-012**: Cross-system tests MUST verify a complete sign request round-trip through the headscale mesh (host daemon → phone → signature returned)

### Key Entities

- **Finding**: A bug discovered by the explore agent — screen, flow, steps to reproduce, expected vs actual, screenshot path, status (new/fixed/verified_broken)
- **Explore Agent**: Agent with MCP tools that walks the app and compares against spec
- **Fix Agent**: Agent with source code access (no MCP) that fixes bugs from findings
- **Verify Agent**: Agent with MCP tools that re-checks fixed bugs on the rebuilt app

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: All 7 screens from UI_FLOW.md are visited and validated on the live emulator with screenshot evidence
- **SC-002**: All navigation flows from the flowchart are exercised end-to-end on the live emulator
- **SC-003**: A sign request round-trip (host → phone → signature) completes successfully through the headscale mesh
- **SC-004**: Error paths produce the exact error messages specified in the Field Validation Reference Table (verified on live emulator)
- **SC-005**: App state survives force-stop/restart (verified via MCP tools on live emulator)
- **SC-006**: The explore-fix-verify loop detects a known-introduced bug, fixes it, and verifies the fix in one cycle

## Assumptions

- The test host has KVM access for acceptable emulator performance
- The `nix-mcp-debugkit` project's `mcp-android` MCP server is available and functional
- The Nix flake provides a working emulator via `start-emulator` in the devshell
- Debug builds include software keystore fallback (no hardware TEE required)
- The deep link bypass (`nix-key://pair?payload=<base64>`) is implemented in debug builds
- The parallel runner handles emulator boot, APK build+install, MCP server lifecycle, and the explore-fix-verify loop — no custom orchestration code is needed
- Biometric authentication can be simulated via emulator fingerprint enrollment
- Pre-authorized Tailscale auth keys can be generated programmatically from headscale

## Non-Goals

- Building custom test orchestration scripts, scenario runners, or prompt templates — the parallel runner already handles this
- Writing shell scripts that invoke future agents — agents use MCP tools directly
- Replacing or refactoring the existing `test/e2e/android_e2e_test.sh` — it continues to work as-is for CI
- Testing on physical devices — emulator only
- iOS or web testing — Android only
