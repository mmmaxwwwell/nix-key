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

- [x] T001 Verify emulator boots, APK installs, and MCP tools respond [FR-001, FR-002, FR-003]
  Done when: `start-emulator` boots emulator, `adb shell getprop sys.boot_completed` returns 1,
  `make android-apk` builds debug APK successfully, `adb install` installs it on the emulator,
  app launches without crash, MCP Screenshot returns a valid PNG, MCP DumpHierarchy returns
  XML with UI elements. `validate/` directory added to .gitignore.

- [x] T002 Verify test bypass mechanisms [FR-007]
  Done when: deep link pairing (`adb shell am start -a android.intent.action.VIEW -d "nix-key://pair?payload=..."`)
  reaches the pairing flow; fingerprint simulation (`adb -e emu finger touch 1`) triggers biometric callback;
  debug APK uses software keystore (no StrongBox requirement).

- [x] T003 Verify headscale/tailscale/daemon infrastructure [FR-006]
  Done when: headscale starts on localhost:18080, host tailscale joins mesh,
  nix-key daemon exposes SSH_AUTH_SOCK, pre-auth keys are generated via `headscale preauthkeys create`.
  Patterns verified against existing `test/e2e/android_e2e_test.sh`.

**Checkpoint**: All infrastructure functional. Runner can boot emulator, provide MCP tools, and set up headscale mesh.

---

## Phase 2: Screen and Flow Validation (US1 — P1)

**Purpose**: Agent explores each screen and navigation flows on the live emulator.
Each screen gets its own E2E task for tight research→fix→verify cycles.

- [x] T004 Validate TailscaleAuth screen [needs: mcp-android, e2e-loop] [FR-004, SC-001]
  Done when: agent has visited TailscaleAuth screen on the live emulator;
  Screenshot verification of layout elements (logo, title, subtitle, auth key field, Connect button,
  "or" divider, "Sign in with Tailscale" button, connection indicator);
  tested: auth key validation (valid tskey-auth-*/tskey-*, invalid, empty, whitespace),
  connection indicator initial state (red/Disconnected), back navigation exits app;
  findings.json has pass/fail entries for TailscaleAuth.

- [x] T004b Validate ServerList screen [needs: mcp-android, e2e-loop] [FR-004, SC-001]
  Done when: agent has navigated to ServerList (via valid auth key on TailscaleAuth);
  Screenshot verification of empty-state layout (title, connection indicator, gear icon,
  illustration, "No paired hosts yet", "Scan QR Code" button);
  tested: connection indicator after auth (yellow/Connecting), gear → Settings navigation,
  "Scan QR Code" → Pairing navigation, back navigation exits app;
  findings.json has pass/fail entries for ServerList.

- [x] T005 Validate Pairing screen [needs: mcp-android, e2e-loop] [FR-004, SC-001]
  Done when: agent has visited Pairing screen via deep link and QR button;
  verified: pairing phases (scanning, connecting, success, error), Cancel/Done buttons,
  error messages for malformed payloads; back navigation to ServerList;
  findings.json has pass/fail for Pairing screen.

- [x] T006 Validate Settings screen [needs: mcp-android, e2e-loop] [FR-004, SC-001]
  Done when: agent has visited Settings screen; verified: Security section (toggles, dropdowns
  with correct labels/defaults per UI_FLOW.md), Tailscale section (IP, tailnet, re-authenticate),
  Tracing section (toggle, OTEL endpoint validation), About section (version, build, licenses);
  back navigation; findings.json has pass/fail for Settings screen.

- [x] T007 Validate KeyManagement + KeyDetail screens [needs: mcp-android, e2e-loop] [FR-004, SC-001]
  Done when: agent has paired a host (via deep link with PHONE_AUTH_KEY), created a key,
  visited KeyManagement list and KeyDetail screens; verified: key creation, name editing,
  policy dropdowns, delete with confirmation, unlock state, back navigation;
  findings.json has pass/fail for both screens.

- [x] T008 Validate SignRequestDialog [needs: mcp-android, e2e-loop] [FR-004, SC-001]
  Done when: agent has triggered a sign request from the host (via ssh-keygen -Y sign
  through SSH_AUTH_SOCK) and observed the overlay dialog via MCP; verified: approve/deny
  buttons, biometric simulation, dialog dismissal; findings.json has pass/fail.

- [x] T009 Validate all navigation flows from UI_FLOW.md flowchart [needs: mcp-android, e2e-loop] [FR-005, SC-002]
  Done when: agent has exercised every navigation edge: first-launch (TailscaleAuth → ServerList
  with back stack cleared), ServerList → Pairing → ServerList, ServerList → KeyManagement →
  KeyDetail → KeyManagement → ServerList, ServerList → Settings → ServerList; back navigation
  verified at each step; findings.json has pass/fail per flow.

**Checkpoint**: SC-001 and SC-002 satisfied. All screens and flows validated on live emulator.

---

## Phase 3: Cross-System Sign Round-Trip (US2 — P1)

**Purpose**: Verify the core product flow — SSH signing via phone through headscale mesh.

- [x] T010 Verify sign approval round-trip [needs: mcp-android, e2e-loop] [FR-012, FR-007, SC-003]
  Done when: headscale mesh running with daemon and emulator app paired via deep link;
  `ssh-keygen -Y sign` triggered from host; agent observes sign dialog via MCP Screenshot +
  WaitForElement within 5 seconds; agent taps Approve; biometric simulated via fingerprint;
  SSH operation exits 0 with valid signature file.

- [x] T011 Verify sign denial round-trip [needs: mcp-android, e2e-loop] [FR-012, SC-003]
  Done when: sign request triggered from host; agent observes dialog via MCP;
  agent taps Deny; SSH operation fails with SSH_AGENT_FAILURE; dialog dismissed.

**Checkpoint**: SC-003 satisfied. Core product flow works end-to-end through headscale mesh.

---

## Phase 4: Error Paths + Persistence (US3, US4 — P2)

**Purpose**: Validate error handling and state persistence on the live emulator.

- [x] T012 Validate error paths from Field Validation Reference Table [needs: mcp-android, e2e-loop] [FR-008, SC-004]
  Done when: agent has entered every invalid input from the Field Validation table via MCP
  Type/SetText and verified exact error message text via DumpHierarchy:
  - TailscaleAuth: invalid key → "Invalid auth key format"
  - KeyDetail: 65-char name → "Name must be 1-64 characters..."
  - KeyDetail: duplicate name → "A key with this name already exists"
  - Settings: invalid OTEL → "Invalid endpoint format (expected host:port)"
  - Pairing: malformed deep link → "Invalid QR code" or "Not a nix-key pairing code"
  All error messages match spec text exactly in findings.json.

- [x] T013 Validate persistence across force-stop/restart [needs: mcp-android, e2e-loop] [FR-011, SC-005]
  Done when: agent creates state (paired host + key), force-stops app via
  `adb shell am force-stop com.nixkey`, restarts app, verifies via DumpHierarchy:
  host still in ServerList, key still in KeyManagement, key shows locked state
  (unlock resets per spec).

**Checkpoint**: SC-004 and SC-005 satisfied. Error messages match spec, state survives restart.

---

## Phase 5: Post-Task Validation

**Purpose**: Verify all builds pass, tests are green, lint is clean, no regressions from fix agent changes.

- [ ] T014 [P] Go build and test validation
  Done when: `make test` passes (unit + integration), `make build` produces nix-key binary.
  Fix any failures introduced by fix-agent source changes. Fix-validate loop, 20-iteration cap.

- [ ] T015 [P] Android build and test validation
  Done when: `make android-apk` produces debug APK, APK installs and launches on emulator,
  `./gradlew testDebugUnitTest` passes in `android/`. Fix any failures. Fix-validate loop, 20-iteration cap.

- [x] T016 [P] Lint validation
  Done when: `make lint` passes (golangci-lint + nixfmt). Fix any new warnings.
  Fix-validate loop, 20-iteration cap.

- [ ] T017 [P] Security scan validation
  Done when: `make security-scan` completes with no new findings introduced by fix-agent changes.
  Fix-validate loop, 20-iteration cap.

- [ ] T018 Existing E2E gate validation
  Done when: `test/e2e/android_e2e_test.sh --skip-build` passes. The existing CI regression
  test still works after any source changes from the fix agent. Fix-validate loop, 10-iteration cap.

- [ ] T019 CI verification [needs: gh, ci-loop]
  Done when: push to remote, CI passes on all jobs (lint, test-host, test-android, security).
  Fix CI failures in a loop. Requires all T014-T018 complete first.

**Checkpoint**: All builds green, all tests pass, lint clean, security clean, CI green.

---

## Phase 6: PR Preparation

**Purpose**: Create PR with E2E findings and any fixes applied.

- [ ] T020 Create PR to develop
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
- **Phase 5** (Post-Task Validation): Depends on Phase 4 (or last phase with source changes)
- **Phase 6** (PR): Depends on Phase 5

### Parallelism

- T014, T015, T016, T017 are marked `[P]` — they target different build systems and can run concurrently
- ALL MCP tasks (T004-T013) are strictly sequential — single emulator
- T018 depends on T014-T017 (needs all builds passing first)
- T019 depends on T018

### Single Emulator Constraint

Phases 2-4 share one Android emulator. No MCP tasks run in parallel. Only Phase 5 build/lint/scan tasks can be parallelized (they don't need the emulator).
