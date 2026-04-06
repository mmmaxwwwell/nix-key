# Tasks: E2E Android Testing

**Input**: spec.md, plan.md, research.md
**Approach**: Fix-validate loop. Each phase: build → test → lint → security scan → read test-logs/ failures → fix code → re-run until green.

**CRITICAL**: Tasks do NOT create shell scripts, prompt templates, scenario runners, or orchestration code. Agents receive MCP tools from the runner and use them directly against the live emulator.

## Format: `[ID] Description`

- `[needs: mcp-android, e2e-loop]` = runner provides MCP tools + explore-fix-verify cycle
- `[P]` = can run in parallel (NEVER used with MCP tasks — single emulator)
- All MCP tasks are sequential (single emulator constraint)

---

## Phase 1: Infrastructure Prerequisites

**Purpose**: Verify the runner's infrastructure works before E2E exploration begins.

- [ ] T001 Verify emulator boots and MCP tools respond [FR-001, FR-003]
  Done when: `start-emulator` boots emulator, `adb shell getprop sys.boot_completed` returns 1,
  MCP Screenshot returns a valid PNG, MCP DumpHierarchy returns XML with UI elements.

- [ ] T002 Verify test bypass mechanisms [FR-007]
  Done when: deep link pairing (`adb shell am start -a android.intent.action.VIEW -d "nix-key://pair?payload=..."`)
  reaches the pairing flow; fingerprint simulation (`adb -e emu finger touch 1`) triggers biometric callback;
  debug APK uses software keystore (no StrongBox requirement).

- [ ] T003 Verify headscale/tailscale/daemon infrastructure [FR-006]
  Done when: headscale starts on localhost:18080, host tailscale joins mesh,
  nix-key daemon exposes SSH_AUTH_SOCK, pre-auth keys are generated via `headscale preauthkeys create`.
  Patterns verified against existing `test/e2e/android_e2e_test.sh`.

**Checkpoint**: All infrastructure functional. Runner can boot emulator, provide MCP tools, and set up headscale mesh.

---

## Phase 2: Screen and Flow Validation (US1 — P1)

**Purpose**: Agent explores all 7 screens and navigation flows on the live emulator.

- [ ] T004 Validate all 7 screens against UI_FLOW.md [needs: mcp-android, e2e-loop] [FR-004, SC-001]
  Done when: agent has visited TailscaleAuth, ServerList, Pairing, KeyManagement, KeyDetail,
  SignRequestDialog, and Settings screens on the live emulator; each screen has a Screenshot
  and DumpHierarchy verification of layout elements from UI_FLOW.md; findings.json has
  pass/fail entries for all 7 screens with screenshot evidence.

- [ ] T005 Validate all navigation flows from UI_FLOW.md flowchart [needs: mcp-android, e2e-loop] [FR-005, SC-002]
  Done when: agent has exercised every navigation edge: first-launch (TailscaleAuth → ServerList
  with back stack cleared), ServerList → Pairing → ServerList, ServerList → KeyManagement →
  KeyDetail → KeyManagement → ServerList, ServerList → Settings → ServerList; back navigation
  verified at each step; findings.json has pass/fail per flow.

**Checkpoint**: SC-001 and SC-002 satisfied. All screens and flows validated on live emulator.

---

## Phase 3: Cross-System Sign Round-Trip (US2 — P1)

**Purpose**: Verify the core product flow — SSH signing via phone through headscale mesh.

- [ ] T006 Verify sign approval round-trip [needs: mcp-android, e2e-loop] [FR-012, FR-007, SC-003]
  Done when: headscale mesh running with daemon and emulator app paired via deep link;
  `ssh-keygen -Y sign` triggered from host; agent observes sign dialog via MCP Screenshot +
  WaitForElement within 5 seconds; agent taps Approve; biometric simulated via fingerprint;
  SSH operation exits 0 with valid signature file.

- [ ] T007 Verify sign denial round-trip [needs: mcp-android, e2e-loop] [FR-012, SC-003]
  Done when: sign request triggered from host; agent observes dialog via MCP;
  agent taps Deny; SSH operation fails with SSH_AGENT_FAILURE; dialog dismissed.

**Checkpoint**: SC-003 satisfied. Core product flow works end-to-end through headscale mesh.

---

## Phase 4: Error Paths + Persistence (US3, US4 — P2)

**Purpose**: Validate error handling and state persistence on the live emulator.

- [ ] T008 Validate error paths from Field Validation Reference Table [needs: mcp-android, e2e-loop] [FR-008, SC-004]
  Done when: agent has entered every invalid input from the Field Validation table via MCP
  Type/SetText and verified exact error message text via DumpHierarchy:
  - TailscaleAuth: invalid key → "Invalid auth key format"
  - KeyDetail: 65-char name → "Name must be 1-64 characters..."
  - KeyDetail: duplicate name → "A key with this name already exists"
  - Settings: invalid OTEL → "Invalid endpoint format (expected host:port)"
  - Pairing: malformed deep link → "Invalid QR code" or "Not a nix-key pairing code"
  All error messages match spec text exactly in findings.json.

- [ ] T009 Validate persistence across force-stop/restart [needs: mcp-android, e2e-loop] [FR-011, SC-005]
  Done when: agent creates state (paired host + key), force-stops app via
  `adb shell am force-stop com.nixkey`, restarts app, verifies via DumpHierarchy:
  host still in ServerList, key still in KeyManagement, key shows locked state
  (unlock resets per spec).

**Checkpoint**: SC-004 and SC-005 satisfied. Error messages match spec, state survives restart.

---

## Phase 5: Explore-Fix-Verify Loop (US5 — P3)

**Purpose**: Full cycle — explore finds bugs, fix agent patches source, runner rebuilds, verify agent confirms.

- [ ] T010 Run explore-fix-verify loop [needs: mcp-android, e2e-loop] [FR-009, FR-010, SC-006]
  Done when: if any prior phase recorded bugs in findings.json, the runner triggers the
  fix-verify cycle: fix agent reads findings.json and modifies source code (any area:
  Kotlin, Go, Nix, proto); runner rebuilds APK and reinstalls; verify agent re-checks
  each bug via MCP tools; loop until all bugs resolved or max 3 iterations.
  findings.json shows zero open bugs (or remaining bugs documented with "verified_broken").

**Checkpoint**: SC-006 satisfied. Explore-fix-verify loop functional.

---

## Phase 6: Post-Task Validation

**Purpose**: Verify all builds pass, tests are green, lint is clean, no regressions from fix agent changes.

- [ ] T011 [P] Go build and test validation
  Done when: `make test` passes (unit + integration), `make build` produces nix-key binary.
  Fix any failures introduced by fix-agent source changes. Fix-validate loop, 20-iteration cap.

- [ ] T012 [P] Android build and test validation
  Done when: `make android-apk` produces debug APK, APK installs and launches on emulator,
  `./gradlew testDebugUnitTest` passes in `android/`. Fix any failures. Fix-validate loop, 20-iteration cap.

- [ ] T013 [P] Lint validation
  Done when: `make lint` passes (golangci-lint + nixfmt). Fix any new warnings.
  Fix-validate loop, 20-iteration cap.

- [ ] T014 [P] Security scan validation
  Done when: `make security-scan` completes with no new findings introduced by fix-agent changes.
  Fix-validate loop, 20-iteration cap.

- [ ] T015 Existing E2E gate validation
  Done when: `test/e2e/android_e2e_test.sh --skip-build` passes. The existing CI regression
  test still works after any source changes from the fix agent. Fix-validate loop, 10-iteration cap.

- [ ] T016 CI verification [needs: gh, ci-loop]
  Done when: push to remote, CI passes on all jobs (lint, test-host, test-android, security).
  Fix CI failures in a loop. Requires all T011-T015 complete first.

**Checkpoint**: All builds green, all tests pass, lint clean, security clean, CI green.

---

## Phase 7: PR Preparation

**Purpose**: Create PR with E2E findings and any fixes applied.

- [ ] T017 Create PR to develop
  Done when: all changes committed, PR created with summary of E2E findings
  (screens validated, bugs found and fixed, sign round-trip verified),
  CI green, ready for review.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1** (Infrastructure): No dependencies — foundational
- **Phase 2** (Screens/Flows): Depends on Phase 1
- **Phase 3** (Sign Round-Trip): Depends on Phase 2
- **Phase 4** (Error Paths/Persistence): Depends on Phase 3
- **Phase 5** (Explore-Fix-Verify): Depends on Phases 2, 3, 4
- **Phase 6** (Post-Task Validation): Depends on Phase 5 (or last phase with source changes)
- **Phase 7** (PR): Depends on Phase 6

### Parallelism

- T011, T012, T013, T014 are marked `[P]` — they target different build systems and can run concurrently
- ALL MCP tasks (T004-T010) are strictly sequential — single emulator
- T015 depends on T011-T014 (needs all builds passing first)
- T016 depends on T015

### Single Emulator Constraint

Phases 2-5 share one Android emulator. No MCP tasks run in parallel. Only Phase 6 build/lint/scan tasks can be parallelized (they don't need the emulator).
