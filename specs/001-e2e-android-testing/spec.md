# Feature Specification: Comprehensive E2E Integration Testing for nix-key Android App

**Feature Branch**: `001-e2e-android-testing`  
**Created**: 2026-04-05  
**Status**: Draft  
**Input**: User description: "Comprehensive E2E Integration Testing for nix-key Android App"

## Clarifications

### Session 2026-04-05

- Q: What is the relationship between CI and the explore-fix-verify loop? → A: CI runs the E2E tests as standard pass/fail assertions. The explore-fix-verify loop is a separate local-only development tool for surfacing and fixing bugs — it is not a test mode and does not run in CI.
- Q: Should P1/P2 tests use mock Tailscale state or real headscale? → A: All tests use a real headscale mesh. The setup cost (~30-60s) is small relative to the 60-minute time budget, CI already has the infrastructure, and this avoids building/maintaining a mock Tailscale layer.
- Q: Should fix agents in the explore-fix-verify loop be scoped to Android code only? → A: No. Fix agents can modify any source code (Android, Go, Nix, proto) if the bug traces to it.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Agent-driven visual E2E test execution (Priority: P1)

A developer (or CI system) launches the E2E test suite, which boots an Android emulator, builds and installs the debug APK, starts the MCP-android server, and runs explore agents that walk every screen and flow defined in UI_FLOW.md. The agents use MCP tools (screenshot, tap, view tree, swipe) to interact with the real app and compare actual behavior against the specification.

**Why this priority**: This is the core capability — without visual agent-driven testing against a real emulator, the feature has no value. It replaces the existing shallow bash orchestrator and mocked Compose tests with real, comprehensive E2E coverage.

**Independent Test**: Can be fully tested by launching the test runner against the emulator with a freshly built APK. The agents navigate every screen, verify visual elements match spec, and produce a structured pass/fail report. Delivers value immediately as a comprehensive regression suite.

**Acceptance Scenarios**:

1. **Given** a clean Android emulator (API 34, x86_64) and freshly built debug APK, **When** the E2E test suite is launched, **Then** the emulator boots, APK is installed, MCP-android server starts, and agents begin navigating the app within 5 minutes of launch.
2. **Given** the test suite is running, **When** agents explore the Tailscale Auth screen, **Then** they verify all layout elements (logo, auth key field, OAuth button, loading states) match UI_FLOW.md and field validations match the validation reference table.
3. **Given** the test suite is running, **When** agents complete all screen explorations, **Then** a structured report is produced listing every screen, every element checked, and pass/fail status for each acceptance scenario.

---

### User Story 2 - Full navigation flow coverage (Priority: P1)

Agents navigate every flow defined in UI_FLOW.md: first launch (Tailscale auth), pairing via QR code (using deep link bypass), key creation and management, sign request approval/denial, and settings configuration. Each flow is exercised end-to-end on the real app.

**Why this priority**: Navigation flows are the backbone of user experience. Broken navigation means a broken app, regardless of whether individual screens render correctly.

**Independent Test**: Can be tested by running the navigation flow agent against the emulator. Each flow is exercised independently: first launch flow, pairing flow, key management flow, sign request flow, settings flow. Each produces its own pass/fail result.

**Acceptance Scenarios**:

1. **Given** a fresh app install with no prior state, **When** the first-launch flow agent runs, **Then** the app shows Tailscale Auth screen, accepts a pre-authorized auth key, navigates to Server List, and the back stack is cleared (pressing back exits the app).
2. **Given** the app is authenticated with Tailscale, **When** the pairing flow agent runs using the deep link bypass (`nix-key://pair?payload=<base64>`), **Then** the pairing completes, the host appears in Server List, and the agent can navigate to Key Management for that host.
3. **Given** a paired host exists, **When** the key management flow agent runs, **Then** it creates a new Ed25519 key, verifies it appears in the key list, navigates to Key Detail, edits the key name, saves, and verifies the updated name persists.

---

### User Story 3 - State machine transition coverage (Priority: P2)

Agents verify every state machine transition defined in UI_FLOW.md: key lifecycle (Creating -> Active -> Editing -> ConfirmDelete -> Deleted), sign request lifecycle (Received -> PromptShown -> AuthChallenge -> Signing -> Completed), Tailscale connection state, and pairing session state.

**Why this priority**: State machines govern the core security model (unlock/sign policies). Incorrect transitions could silently compromise key security or leave the app in an unrecoverable state.

**Independent Test**: Each state machine can be tested independently. For example, the key lifecycle agent creates a key, edits it, deletes it with biometric confirmation, and verifies each intermediate state via the UI.

**Acceptance Scenarios**:

1. **Given** no keys exist, **When** the key lifecycle agent creates a key with biometric unlock policy, **Then** the key transitions through Creating -> Active, and the lock indicator shows "locked" in the Key Management list.
2. **Given** a sign request arrives for a locked key, **When** the sign request lifecycle agent observes the UI, **Then** the unlock prompt appears first, followed by the sign approval dialog, matching the two-step flow described in UI_FLOW.md.
3. **Given** Tailscale is connected, **When** the Tailscale connection agent simulates a network disconnection (via emulator network control), **Then** the Tailnet indicator transitions from green/"Connected" to red/"Disconnected" and back to green/"Connected" when connectivity is restored.

---

### User Story 4 - Error path and edge case validation (Priority: P2)

Agents systematically exercise every error path: invalid Tailscale auth keys, malformed QR codes, mTLS certificate mismatches, biometric authentication failures, port conflicts, stale auth states, and network timeouts. Each error path produces the correct user-facing error message.

**Why this priority**: Error handling is where security tools fail silently. The existing shallow tests never exercise error paths, which is where the most dangerous bugs hide.

**Independent Test**: Each error category can be tested independently by injecting the specific failure condition and verifying the error UI matches specification.

**Acceptance Scenarios**:

1. **Given** the Tailscale Auth screen is shown, **When** an agent enters an invalid auth key (e.g., "not-a-real-key"), **Then** the validation error "Invalid auth key format" is displayed inline.
2. **Given** the Pairing screen is active, **When** the agent triggers a deep link with a malformed QR payload (missing required fields), **Then** the error message "Invalid QR code" or "Not a nix-key pairing code" is shown.
3. **Given** a sign request arrives, **When** the user denies biometric authentication, **Then** the dialog is dismissed and SSH_AGENT_FAILURE is returned (verified by the test harness monitoring gRPC responses).

---

### User Story 5 - Explore-fix-verify loop with bug remediation (Priority: P2)

When agents discover a bug (UI element missing, wrong error message, broken navigation, etc.), they batch the issues, apply fixes to the source code, rebuild and reinstall the APK, and re-verify each fix. A supervisor agent reviews progress periodically and adjusts strategy.

**Why this priority**: The explore-fix-verify loop is what makes this E2E suite actionable rather than just a passive test reporter. Without it, bugs are reported but not fixed, requiring manual developer intervention.

**Independent Test**: Can be tested by intentionally introducing a known bug (e.g., wrong error message text), running the explore-fix-verify loop, and verifying the agent detects the bug, fixes it, rebuilds, and confirms the fix.

**Acceptance Scenarios**:

1. **Given** agents have discovered 3 UI bugs during exploration, **When** the fix phase runs, **Then** the agents batch-fix all 3 bugs in source code, rebuild the APK, reinstall, and verify each fix independently.
2. **Given** a fix has been applied and verified, **When** the supervisor reviews after 10 iterations, **Then** it produces a progress report listing bugs found, fixed, verified, and remaining.
3. **Given** a fix introduces a regression in another flow, **When** the verify phase runs, **Then** the regression is detected, reported, and the fix is revised in the next cycle.

---

### User Story 6 - Cross-system gRPC integration testing (Priority: P3)

The E2E suite includes tests that exercise the full host-to-phone communication path: the nix-key daemon on the host sends a sign request via gRPC over mTLS through Tailscale (using the existing headscale test infrastructure), the phone app receives it, shows the sign prompt, and returns the signature.

**Why this priority**: Cross-system integration validates the entire end-to-end path that the product is built for. However, it requires complex infrastructure (headscale, daemon, mTLS) and builds on top of the single-app E2E tests.

**Independent Test**: Can be tested by starting the headscale mesh, nix-key daemon, and Android emulator, then triggering a sign request from the host and observing the full round-trip through the phone UI.

**Acceptance Scenarios**:

1. **Given** headscale mesh is running with the nix-key daemon and Android emulator paired, **When** the host triggers a sign request, **Then** the sign request dialog appears on the phone emulator within 5 seconds.
2. **Given** a sign request dialog is shown on the emulator, **When** the agent taps Approve and biometric auth succeeds, **Then** the signature is returned to the host daemon and the SSH operation completes successfully.

---

### User Story 7 - Persistence and recovery testing (Priority: P3)

Agents test that app state persists correctly across restarts: paired hosts survive app restart, keys survive process kill, unlock state resets on app stop (as specified), and stale Tailscale auth triggers re-authentication.

**Why this priority**: Persistence bugs cause data loss and are especially dangerous for a security tool where losing pairing state means re-pairing all devices.

**Independent Test**: Can be tested by creating state (pair a host, create a key), force-stopping the app, restarting, and verifying state integrity.

**Acceptance Scenarios**:

1. **Given** a host is paired and a key exists, **When** the app process is killed and restarted, **Then** the host still appears in Server List and the key still appears in Key Management.
2. **Given** a key is in unlocked state, **When** the app is stopped and restarted, **Then** the key returns to locked state (unlock state resets per spec).
3. **Given** the Tailscale auth token has expired, **When** the app is launched, **Then** the re-authentication flow is shown instead of crashing (FR-115).

---

### Edge Cases

- What happens when the emulator runs out of disk space during APK installation?
- How does the test suite handle an emulator that fails to boot within the timeout period?
- What happens when MCP-android loses connection to the emulator mid-test?
- How does the suite handle a test that never terminates (infinite loop in app UI)?
- What happens when multiple sign requests arrive simultaneously while the app is backgrounded?
- How does the app behave when Tailscale VPN is connected but the gRPC port is blocked?
- What happens when the key name uniqueness constraint is violated during rapid key creation?

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST boot an Android emulator and verify it is responsive before proceeding with tests
- **FR-002**: System MUST build the debug APK from source and install it on the emulator before each test run
- **FR-003**: System MUST start the MCP-android server and verify it can communicate with the emulator
- **FR-004**: System MUST provide explore agents that navigate every screen defined in UI_FLOW.md (7 screens: TailscaleAuth, ServerList, Pairing, KeyManagement, KeyDetail, SignRequestDialog, Settings)
- **FR-005**: System MUST verify all layout elements on each screen match the UI_FLOW.md specification (field presence, labels, button text, indicators)
- **FR-006**: System MUST exercise every navigation flow defined in the navigation flowchart (first launch, pairing, key management, sign request, settings)
- **FR-007**: System MUST provide test bypass mechanisms for QR code scanning (deep link with payload), biometric authentication (emulator test biometric or debug auto-approve), and hardware keystore (software keystore fallback). Tailscale authentication uses a real headscale mesh with pre-authorized auth keys (not mocked)
- **FR-008**: System MUST verify every state machine transition defined in UI_FLOW.md for key lifecycle, sign request lifecycle, Tailscale connection, and pairing session states
- **FR-009**: System MUST exercise error paths including invalid auth keys, malformed QR payloads, biometric failures, and network timeouts, verifying correct error messages are displayed
- **FR-010**: System MUST produce structured test output (pass/fail per scenario, screenshots on failure, timing data) in JSON format aggregatable by `scripts/ci-summary.sh` into ci-summary.json
- **FR-011**: System MUST support an explore-fix-verify loop as a local development tool where discovered bugs are batched, fixed in any source code the bug traces to (Android, Go, Nix, proto), rebuilt and reinstalled, and fixes are verified. This is separate from the test suite and does not run in CI.
- **FR-012**: System MUST validate field validations from the Field Validation Reference Table (auth key format, key name constraints, OTEL endpoint format, QR payload structure)
- **FR-013**: System MUST test persistence across app restart: paired hosts, created keys, and settings survive process kill; unlock state resets on app stop
- **FR-014**: System MUST support cross-system integration tests using the existing headscale + nix-key daemon infrastructure for full host-to-phone gRPC round-trips
- **FR-015**: System MUST include a supervisor agent in the local explore-fix-verify loop that reviews progress periodically and adjusts strategy based on findings. The supervisor does not run in CI mode.
- **FR-016**: System MUST integrate with the existing CI/CD pipeline for automated E2E test execution as standard pass/fail assertions

### Key Entities

- **Test Suite**: The complete collection of E2E tests organized by screen, flow, and state machine. Contains test scenarios, bypass configurations, and output settings.
- **Explore Agent**: An autonomous agent that navigates the app using MCP tools, compares actual behavior against spec, and reports discrepancies.
- **Fix Agent**: An agent that receives bug reports from explore agents, applies source code fixes in batches, and triggers rebuilds.
- **Supervisor Agent**: A periodic reviewer that assesses overall progress, identifies stuck agents, and adjusts strategy.
- **Test Bypass**: A mechanism that substitutes production behavior (QR scan, biometric, Tailscale auth, hardware keystore) with test-compatible alternatives during E2E runs.
- **MCP-Android Server**: The server that provides visual and interactive access to the emulator for agents (Screenshot, DumpHierarchy, Click, Swipe, Type, etc.).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: All 7 screens defined in UI_FLOW.md are visited and validated by explore agents, with 100% element coverage per screen
- **SC-002**: All navigation flows from the navigation flowchart are exercised end-to-end, including back navigation and back-stack behavior
- **SC-003**: Every state machine transition in UI_FLOW.md is covered by at least one test scenario
- **SC-004**: Error paths produce the exact error messages specified in the Field Validation Reference Table
- **SC-005**: The explore-fix-verify loop can detect a known-introduced bug, fix it, rebuild, and verify the fix within a single cycle
- **SC-006**: The full test suite completes within 60 minutes on a CI machine with hardware virtualization support
- **SC-007**: Test output includes structured pass/fail results, failure screenshots, and timing data aggregatable by `scripts/ci-summary.sh` into ci-summary.json
- **SC-008**: Cross-system integration tests verify a complete sign request round-trip (host daemon to phone to signature returned) through the test mesh network
- **SC-009**: Persistence tests verify state integrity across at least one app process kill-restart cycle

## Assumptions

- The test host has hardware virtualization support for acceptable emulator performance
- The `nix-mcp-debugkit` project's `mcp-android` MCP server is available and functional with the required tools (Screenshot, DumpHierarchy, Click, ClickBySelector, Swipe, Type, SetText, Press, WaitForElement, GetScreenInfo)
- The existing emulator Nix package provides a working emulator with software rendering
- Debug builds of the Android app include a software keystore fallback that does not require hardware-backed TEE/StrongBox
- The deep link bypass (`nix-key://pair?payload=<base64>`) is implemented and functional in debug builds
- The existing headscale + Tailscale test infrastructure from NixOS VM tests is used for all E2E tests (not just cross-system), providing real Tailscale networking throughout
- Tailscale pre-authorized auth keys can be generated programmatically for test automation
- The spec-kit parallel runner supports `[needs: mcp-android, e2e-loop]` task annotations for agent orchestration
- Biometric authentication can be simulated on the emulator via test biometric enrollment or a debug auto-approve flag
