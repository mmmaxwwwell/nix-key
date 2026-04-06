# Tasks: Comprehensive E2E Integration Testing

**Input**: Design documents from `/specs/001-e2e-android-testing/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: Test tasks are included — this feature IS test infrastructure (constitution principle III: Test-First).

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Create the directory structure and shared library scaffolding

- [x] T001 Create test/e2e/scenarios/ directory and test/e2e/prompts/ directory and test/e2e/lib/ directory
- [ ] T002 Add test-logs/e2e/ to .gitignore (screenshots/, hierarchies/, bugs/, iterations/, supervisor/ subdirectories)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Extract infrastructure from existing orchestrator into reusable library and build the scenario runner framework. MUST be complete before any user story scenarios can run.

**CRITICAL**: No user story work can begin until this phase is complete.

- [ ] T003 Extract headscale setup/teardown functions from test/e2e/android_e2e_test.sh into test/e2e/lib/infrastructure.sh (start_headscale, stop_headscale, create_headscale_user, generate_preauth_keys)
- [ ] T004 Extract tailscale node setup/teardown functions into test/e2e/lib/infrastructure.sh (start_host_tailscale, start_phone_tailscale, join_headscale)
- [ ] T005 Extract emulator management functions into test/e2e/lib/infrastructure.sh (boot_emulator, wait_for_emulator, install_apk, kill_emulator) with timeout handling for boot failures and disk space pre-check
- [ ] T006 Extract nix-key daemon management functions into test/e2e/lib/infrastructure.sh (start_daemon, wait_for_daemon, run_pairing, stop_daemon)
- [ ] T007 Create test/e2e/lib/mcp-helpers.sh with MCP-android server lifecycle (start_mcp_server, stop_mcp_server, wait_for_mcp_ready) including a smoke test that verifies Screenshot returns a valid PNG from the emulator, and reconnection logic if MCP loses connection mid-test
- [ ] T008 Create test/e2e/lib/report.sh implementing structured JSON output per contracts/test-output.md (init_report, add_scenario_result, finalize_report, capture_failure_screenshot, capture_failure_hierarchy)
- [ ] T009 Create test/e2e/lib/scenario-runner.sh that takes a scenario script + infrastructure state, invokes the agent with prompt + spec context, collects structured results, and reports via report.sh
- [ ] T010 Update test/e2e/android_e2e_test.sh to source test/e2e/lib/infrastructure.sh instead of inline functions, add --scenarios, --priority, and --explore-fix-verify flag parsing per contracts/e2e-runner.md
- [ ] T011 Verify refactored test/e2e/android_e2e_test.sh still passes the existing E2E flow (headscale + pairing + sign + deny) end-to-end on emulator

**Checkpoint**: Infrastructure library extracted and validated. Existing E2E flow still works. Scenario runner framework ready.

---

## Phase 3: User Story 1 — Agent-Driven Screen Validation (Priority: P1) MVP

**Goal**: Agents use MCP tools to visit all 7 screens and verify layout elements match UI_FLOW.md.

**Independent Test**: Launch the test runner with `--scenarios="US1-*"`. Agents navigate every screen and produce a pass/fail report with 100% element coverage per screen.

### Implementation for User Story 1

- [ ] T012 [US1] Create test/e2e/prompts/explore-screen.md agent prompt template — context section referencing specs/nix-key/UI_FLOW.md per-screen details, MCP tool usage patterns (Screenshot+DumpHierarchy for observation, ClickBySelector/Type for interaction, WaitForElement for sync), validation criteria (element presence, labels, button text, indicators), structured output format (pass/fail per element)
- [ ] T013 [US1] Create test/e2e/scenarios/us1-screen-validation.sh — defines 7 sub-scenarios (one per screen: TailscaleAuth, ServerList, Pairing, KeyManagement, KeyDetail, SignRequestDialog, Settings), each invoking explore-screen prompt with screen-specific context from UI_FLOW.md, collecting results per screen via scenario-runner.sh
- [ ] T014 [US1] Add TailscaleAuth screen validation to test/e2e/scenarios/us1-screen-validation.sh — verify logo, auth key field (monospace, paste-friendly), OAuth button, loading state ("Connecting to Tailnet..."), connection indicator (yellow/spinner during auth per FR-112), field validation (auth key format per validation table)
- [ ] T015 [US1] Add ServerList screen validation to test/e2e/scenarios/us1-screen-validation.sh — verify "nix-key" title, Tailnet indicator (green/yellow/red per FR-110), gear icon, host rows (name, IP, status dot), empty state illustration, "Scan QR Code" button
- [ ] T016 [US1] Add Pairing screen validation to test/e2e/scenarios/us1-screen-validation.sh — verify camera viewfinder, cancel button, Tailnet indicator, loading states ("Scanning..." per FR-113, "Connecting to host...", "Waiting for host approval..."), result screen (success checkmark or error message)
- [ ] T017 [US1] Add KeyManagement screen validation to test/e2e/scenarios/us1-screen-validation.sh — verify host name in top bar, back arrow, key list rows (lock/unlock indicator per FR-111, key name, type badge, fingerprint, created date), empty state, FAB "+" button
- [ ] T018 [US1] Add KeyDetail screen validation to test/e2e/scenarios/us1-screen-validation.sh — verify create mode (name field, type selector Ed25519/ECDSA-P256, info text, unlock policy picker, signing policy picker, Create button) and view/edit mode (fingerprint, export section with Copy/Share/QR, Delete button, Save button, Lock/Unlock button per FR-117)
- [ ] T019 [US1] Add SignRequestDialog validation to test/e2e/scenarios/us1-screen-validation.sh — trigger sign request via host daemon, verify dialog card (title "Sign Request", host name, key name, data hash, Approve/Deny buttons), verify overlay behavior on different screens
- [ ] T020 [US1] Add Settings screen validation to test/e2e/scenarios/us1-screen-validation.sh — verify Security section (key listing toggle, default unlock policy, default signing policy), Tracing section (toggle, OTEL endpoint), Tailscale section (IP, Tailnet name, Re-authenticate), About section (version, build info, licenses)

**Checkpoint**: All 7 screens validated against UI_FLOW.md. SC-001 satisfied.

---

## Phase 4: User Story 2 — Navigation Flow Coverage (Priority: P1)

**Goal**: Agents exercise every navigation flow from the UI_FLOW.md flowchart end-to-end.

**Independent Test**: Launch with `--scenarios="US2-*"`. Each navigation flow produces its own pass/fail. SC-002 satisfied.

### Implementation for User Story 2

- [ ] T021 [US2] Create test/e2e/prompts/explore-flow.md agent prompt template — context section referencing navigation flowchart from UI_FLOW.md, instructions to exercise forward + back navigation, back-stack verification (press Back, verify expected destination), structured output (pass/fail per flow step)
- [ ] T022 [US2] Create test/e2e/scenarios/us2-navigation-flows.sh — defines sub-scenarios for each navigation flow, invokes explore-flow prompt per flow
- [ ] T023 [US2] Add first-launch flow to test/e2e/scenarios/us2-navigation-flows.sh — fresh install → TailscaleAuth shown → enter pre-auth key via MCP SetText → "Connecting to Tailnet..." loading → navigate to ServerList → verify back stack cleared (Back exits app)
- [ ] T024 [US2] Add pairing flow to test/e2e/scenarios/us2-navigation-flows.sh — ServerList → tap "Scan QR Code" → Pairing screen → trigger deep link with valid payload → confirmation bottom sheet → accept → handshake loading states → success result → "Done" → ServerList with new host
- [ ] T025 [US2] Add key management flow to test/e2e/scenarios/us2-navigation-flows.sh — ServerList → tap host row → KeyManagement → tap FAB → KeyDetail (create mode) → fill name + select Ed25519 + set policies → Create → view mode → edit name → Save → Back → verify in list → tap key → KeyDetail (view) → Back
- [ ] T026 [US2] Add sign request flow to test/e2e/scenarios/us2-navigation-flows.sh — trigger sign request from host daemon → SignRequestDialog overlay appears → tap Approve → biometric challenge (emulator fingerprint) → signature returned → dialog dismissed → verify underlying screen unchanged
- [ ] T027 [US2] Add settings flow to test/e2e/scenarios/us2-navigation-flows.sh — ServerList → tap gear → Settings → toggle key listing → change default policies → Back → ServerList → verify settings persisted by re-entering Settings

**Checkpoint**: All navigation flows exercised. SC-002 satisfied.

---

## Phase 5: User Story 3 — State Machine Transitions (Priority: P2)

**Goal**: Verify every state machine transition from UI_FLOW.md (key lifecycle, sign request, Tailscale connection, pairing session).

**Independent Test**: Launch with `--scenarios="US3-*"`. Each state machine has its own sub-scenario. SC-003 satisfied.

### Implementation for User Story 3

- [ ] T028 [US3] Create test/e2e/prompts/explore-state-machine.md agent prompt template — context section referencing state machine diagrams from UI_FLOW.md, instructions to drive each transition and verify UI reflects expected state, structured output (pass/fail per transition)
- [ ] T029 [US3] Create test/e2e/scenarios/us3-state-machines.sh — defines sub-scenarios for each of the 4 state machines
- [ ] T030 [US3] Add key lifecycle state machine to test/e2e/scenarios/us3-state-machines.sh — Creating (tap Create Key, verify loading) → Active (verify key in list, lock indicator) → Editing (edit name, verify Save enabled) → Active (save) → ConfirmDelete (tap Delete, biometric challenge) → Deleted (verify key removed from list). Also test Failed path (invalid key name)
- [ ] T031 [US3] Add sign request lifecycle to test/e2e/scenarios/us3-state-machines.sh — Received (trigger gRPC sign) → PromptShown (verify dialog) → AuthChallenge (tap Approve, biometric) → Signing → Completed (verify signature returned). Also test: Denied (tap Deny → SSH_AGENT_FAILURE), TimedOut (wait for signTimeout), AuthFailed (fail biometric → SSH_AGENT_FAILURE). Test locked key path: unlock prompt shown first (FR-116), then sign prompt
- [ ] T032 [US3] Add Tailscale connection state machine to test/e2e/scenarios/us3-state-machines.sh — Unauthenticated (first launch) → Authenticating (enter key) → Connected (verify green indicator) → Listening (gRPC server bound). Test Disconnected transition by toggling emulator network via adb (airplane mode on/off), verify indicator red/"Disconnected" → green/"Connected"
- [ ] T033 [US3] Add pairing session state machine to test/e2e/scenarios/us3-state-machines.sh — Scanning (camera active) → QRDecoded (valid deep link) → AwaitingUserConfirm (bottom sheet) → Connecting (accept) → Handshaking → PairingSuccess (verify host in list). Test: user denies → back to ServerList. Test: invalid token → PairingFailed with error message

**Checkpoint**: All 4 state machines fully covered. SC-003 satisfied.

---

## Phase 6: User Story 4 — Error Path Validation (Priority: P2)

**Goal**: Exercise every error path and verify correct error messages per Field Validation Reference Table.

**Independent Test**: Launch with `--scenarios="US4-*"`. Each error category has its own sub-scenario. SC-004 satisfied.

### Implementation for User Story 4

- [ ] T034 [US4] Create test/e2e/prompts/explore-error-path.md agent prompt template — context section referencing Field Validation Reference Table from UI_FLOW.md, instructions to inject each error condition and verify exact error message text, structured output (pass/fail per error path with expected vs actual message)
- [ ] T035 [US4] Create test/e2e/scenarios/us4-error-paths.sh — defines sub-scenarios for each error category
- [ ] T036 [US4] Add Tailscale auth error paths to test/e2e/scenarios/us4-error-paths.sh — invalid key format (no prefix) → "Invalid auth key format", empty key → validation error, whitespace in key → validation error
- [ ] T037 [US4] Add pairing error paths to test/e2e/scenarios/us4-error-paths.sh — malformed QR payload (missing hostIp) → "Invalid QR code", non-Tailscale IP → "Not a nix-key pairing code", invalid port → error, invalid PEM cert → error, empty token → error, token replay → 401 rejection
- [ ] T038 [US4] Add key creation error paths to test/e2e/scenarios/us4-error-paths.sh — empty name → validation error, name >64 chars → "Name must be 1-64 characters...", special characters in name → validation error, duplicate name → "A key with this name already exists"
- [ ] T039 [US4] Add sign request error paths to test/e2e/scenarios/us4-error-paths.sh — deny biometric → SSH_AGENT_FAILURE, sign timeout → SSH_AGENT_FAILURE, key not found → NOT_FOUND gRPC error. Verify dialog dismissed correctly after each
- [ ] T040 [US4] Add settings error paths to test/e2e/scenarios/us4-error-paths.sh — invalid OTEL endpoint format → "Invalid endpoint format (expected host:port)", port out of range → validation error
- [ ] T041 [US4] Add security warning dialog validation to test/e2e/scenarios/us4-error-paths.sh — select "None" unlock policy → security warning dialog with correct title/body/buttons per UI_FLOW.md, cancel → policy unchanged, confirm → policy set. Select "Auto-approve" signing → security warning dialog, cancel, confirm. Verify FR-046 warning badge on keys with none-unlock + auto-approve

- [ ] T041b [US4] Add infrastructure edge case tests to test/e2e/scenarios/us4-error-paths.sh — per-scenario timeout guard (kill agent if scenario exceeds 5 minutes, report as failed with "scenario timeout"), concurrent sign requests (trigger 3 sign requests in rapid succession from host, verify they queue as individual dialogs in arrival order per UI_FLOW.md), blocked gRPC port (block port 29418 via iptables on emulator, verify Tailnet indicator shows red/"Disconnected" and notification shows "Port 29418 in use" per FR-E19), key name race (create 2 keys with same name in rapid succession, verify duplicate name error "A key with this name already exists")

**Checkpoint**: All error paths validated against Field Validation Reference Table. SC-004 satisfied.

---

## Phase 7: User Story 5 — Explore-Fix-Verify Loop (Priority: P2)

**Goal**: Implement the local-only explore-fix-verify loop that discovers bugs, fixes them, rebuilds, and verifies.

**Independent Test**: Introduce a known bug (e.g., wrong error message text in a Kotlin file), run `--explore-fix-verify --max-iterations=3`, verify the bug is detected, fixed, rebuilt, and verified in one cycle. SC-005 satisfied.

### Implementation for User Story 5

- [ ] T042 [US5] Create test/e2e/prompts/fix-bugs.md agent prompt template — receives list of BugReport JSON objects, source code access, instructions to analyze root cause across any codebase area (Android/Go/Nix/proto), batch-fix strategy, output format (list of files modified with diffs)
- [ ] T043 [US5] Create test/e2e/prompts/verify-fixes.md agent prompt template — receives list of fixed BugReports, re-runs the specific failing scenarios via MCP tools, verifies each fix, detects regressions in related flows, output format (verified/regression per bug)
- [ ] T044 [US5] Create test/e2e/prompts/supervisor-review.md agent prompt template — receives iteration history (bugs found/fixed/verified/remaining), assesses progress, identifies stuck patterns, suggests strategy adjustments, output format (progress report + strategy recommendations)
- [ ] T045 [US5] Add explore-fix-verify loop orchestration to test/e2e/android_e2e_test.sh — when --explore-fix-verify flag is set: run explore phase (all scenarios), collect BugReports to test-logs/e2e/bugs/, run fix agent, rebuild APK (build-android-apk), reinstall, run verify agent, run supervisor every N iterations per --supervisor-interval, loop until all verified or --max-iterations reached
- [ ] T046 [US5] Add BugReport JSON output to test/e2e/lib/report.sh — create_bug_report, update_bug_status, write iteration summary per contracts/test-output.md local mode format
- [ ] T047 [US5] Add supervisor output to test/e2e/lib/report.sh — write supervisor review JSON to test-logs/e2e/supervisor/

**Checkpoint**: Explore-fix-verify loop functional. SC-005 satisfied.

---

## Phase 8: User Story 6 — Cross-System gRPC Integration (Priority: P3)

**Goal**: Exercise the full host-to-phone communication path through the headscale mesh.

**Independent Test**: Launch with `--scenarios="US6-*"`. Triggers a sign request from the host daemon, observes the full round-trip. SC-008 satisfied.

**Dependencies**: Requires Phases 2-4 complete (infrastructure + pairing + sign request scenarios).

### Implementation for User Story 6

- [ ] T048 [US6] Create test/e2e/scenarios/us6-cross-system.sh — sets up full cross-system environment (host daemon paired with emulator app via headscale), triggers SSH sign request from host (ssh-keygen -Y sign), verifies sign dialog appears on emulator within 5 seconds via MCP Screenshot+WaitForElement
- [ ] T049 [US6] Add sign approval round-trip to test/e2e/scenarios/us6-cross-system.sh — agent taps Approve on emulator via MCP, biometric challenge via emulator fingerprint simulation, verify signature returned to host daemon, verify SSH operation completes successfully (exit code 0)
- [ ] T050 [US6] Add sign denial round-trip to test/e2e/scenarios/us6-cross-system.sh — agent taps Deny, verify SSH_AGENT_FAILURE returned to host, verify SSH operation fails with expected error
- [ ] T051 [US6] Add Ping RPC verification to test/e2e/scenarios/us6-cross-system.sh — run `nix-key test <device>` from host, verify Ping response received with valid timestamp

**Checkpoint**: Full host-to-phone round-trip verified. SC-008 satisfied.

---

## Phase 9: User Story 7 — Persistence and Recovery (Priority: P3)

**Goal**: Verify app state persists correctly across restarts and recovery scenarios.

**Independent Test**: Launch with `--scenarios="US7-*"`. Creates state, kills app, restarts, verifies. SC-009 satisfied.

### Implementation for User Story 7

- [ ] T052 [US7] Create test/e2e/scenarios/us7-persistence.sh — pair a host, create a key, force-stop app via `adb shell am force-stop com.nixkey`, restart app via `adb shell am start`, verify host still in ServerList and key still in KeyManagement via MCP DumpHierarchy
- [ ] T053 [US7] Add unlock state reset verification to test/e2e/scenarios/us7-persistence.sh — unlock a key (biometric), verify unlocked indicator, force-stop + restart app, verify key shows locked indicator (unlock state resets per spec)
- [ ] T054 [US7] Add stale Tailscale auth recovery to test/e2e/scenarios/us7-persistence.sh — simulate expired auth (clear Tailscale state on emulator via adb), restart app, verify re-authentication flow shown instead of crash (FR-115)

**Checkpoint**: Persistence and recovery validated. SC-009 satisfied.

---

## Phase 10: Polish & Cross-Cutting Concerns

**Purpose**: CI integration, documentation, and final validation

- [ ] T055 Update .github/workflows/e2e.yml to use new scenario-based runner — replace direct `android_e2e_test.sh` call with scenario flags, preserve retry logic (3 attempts, 15min timeout), add test-logs/e2e/ to artifact uploads
- [ ] T056 Update scripts/ci-summary.sh to aggregate test-logs/e2e/summary.json into ci-summary.json per contracts/test-output.md format, and validate that the aggregated output parses correctly (run ci-summary.sh on sample E2E output and verify JSON schema)
- [ ] T057 Validate end-to-end: run full test suite locally with `./test/e2e/android_e2e_test.sh` and verify summary.json output matches contracts/test-output.md schema, all scenarios produce results, screenshots captured on failure, and total wall-clock time is under 60 minutes (SC-006)
- [ ] T058 Validate explore-fix-verify: introduce a known bug, run `--explore-fix-verify --max-iterations=3`, verify detection → fix → rebuild → verify cycle completes
- [ ] T059 Run specs/001-e2e-android-testing/quickstart.md validation — execute each quickstart command and verify expected behavior

---

## Dependencies & Execution Order

### Single Emulator Constraint

**All scenario tasks share one Android emulator.** This means scenario execution is strictly sequential — no user story scenarios can run in parallel. The only parallelizable tasks are prompt-writing tasks (markdown files that don't touch the emulator) and library files that target different output files.

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all user stories
- **US1 Screen Validation (Phase 3)**: Depends on Foundational — MVP
- **US2 Navigation Flows (Phase 4)**: Depends on Foundational — runs after US1
- **US3 State Machines (Phase 5)**: Depends on Foundational — runs after US2
- **US4 Error Paths (Phase 6)**: Depends on Foundational — runs after US3
- **US5 Explore-Fix-Verify (Phase 7)**: Depends on at least US1 complete (needs scenarios to explore)
- **US6 Cross-System (Phase 8)**: Depends on Foundational + pairing/sign infrastructure from US2/US3
- **US7 Persistence (Phase 9)**: Depends on Foundational + pairing/key creation from US2
- **Polish (Phase 10)**: Depends on all user stories complete

### User Story Dependencies

- **US1 (P1)**: Can start after Foundational
- **US2 (P1)**: Runs after US1 (single emulator)
- **US3 (P2)**: Runs after US2 (single emulator)
- **US4 (P2)**: Runs after US3 (single emulator)
- **US5 (P2)**: Requires at least US1 scenarios to exist
- **US6 (P3)**: Runs after US4; requires pairing/sign infrastructure
- **US7 (P3)**: Runs after US6; requires pairing/key creation infrastructure

### Within Each User Story

- Prompt template before scenario script
- Scenario script before sub-scenario implementations
- Sub-scenarios run sequentially (single emulator)

### Limited Parallel Opportunities

Only tasks that write to **different files** and **do not require the emulator** can run in parallel:

- T007 (mcp-helpers.sh) and T008 (report.sh) — different lib files, can be written in parallel
- Prompt-writing tasks across stories can be parallelized IF done before any scenario execution:
  - T012 (explore-screen.md), T021 (explore-flow.md), T028 (explore-state-machine.md), T034 (explore-error-path.md)
  - T042, T043, T044 (fix-bugs.md, verify-fixes.md, supervisor-review.md)
- T003-T006 all write to the same file (infrastructure.sh) — must be sequential

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL — blocks all stories)
3. Complete Phase 3: User Story 1 (screen validation)
4. **STOP and VALIDATE**: Run `--scenarios="US1-*"` — all 7 screens should be validated
5. This alone replaces the shallow existing tests with real visual validation

### Incremental Delivery

1. Setup + Foundational → Infrastructure library ready
2. Add US1 → Screen validation works → MVP
3. Add US2 → Navigation flows covered → Full P1 coverage
4. Add US3 + US4 → State machines + error paths → Full P2 coverage
5. Add US5 → Explore-fix-verify loop → Developer tool ready
6. Add US6 + US7 → Cross-system + persistence → Full P3 coverage
7. Polish → CI integration → Production-ready

### Sequential Execution (Single Emulator)

Since all scenarios share one emulator, execution is sequential:

1. Setup + Foundational (write all lib files)
2. Write all prompt templates (can be done in parallel — no emulator needed)
3. US1 scenarios (screen validation) → validate on emulator
4. US2 scenarios (navigation) → validate on emulator
5. US3 scenarios (state machines) → validate on emulator
6. US4 scenarios (error paths) → validate on emulator
7. US5 (explore-fix-verify loop orchestration)
8. US6 scenarios (cross-system) → validate on emulator
9. US7 scenarios (persistence) → validate on emulator
10. Polish (CI integration)

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- Agent prompts reference specs/nix-key/UI_FLOW.md and specs/nix-key/spec.md as source of truth
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- The existing android_e2e_test.sh must continue to pass after refactoring (T011 validates this)
