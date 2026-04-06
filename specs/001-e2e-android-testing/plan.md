# Implementation Plan: E2E Android Testing

**Feature**: 001-e2e-android-testing  
**Created**: 2026-04-06  
**Spec**: [spec.md](spec.md)  
**Research**: [research.md](research.md)  
**Preset**: public

## Summary

Agents use MCP tools to explore the live nix-key Android app on an emulator, validate against UI_FLOW.md, and fix bugs via the explore-fix-verify loop. The parallel runner handles all infrastructure — emulator boot, APK build+install, MCP server lifecycle, and agent coordination. Tasks do NOT create shell scripts, prompt templates, scenario runners, or any orchestration code.

## Technical Context

- **Languages**: Bash (existing orchestrator), Go 1.22+ (host daemon), Kotlin (Android app)
- **Primary dependency**: `mcp-android` from `nix-mcp-debugkit`
- **Platform**: Linux with KVM, Android emulator API 34 (x86_64)
- **Testing**: Agent-driven via MCP tools (Screenshot, DumpHierarchy, Click, Type, WaitForElement, etc.)
- **Output**: Structured `findings.json` with screenshot evidence
- **Constraint**: Single emulator — all scenarios are sequential

## Constitution Check

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Nix-First | Pass | Emulator via `start-emulator` in devshell, APK via `nix/android-apk.nix` |
| II. Security by Default | Pass | Tests use real mTLS over headscale mesh; bypass mechanisms are debug-only |
| III. Test-First | Pass | This IS test infrastructure |
| IV. Unix Philosophy | Pass | Extends existing patterns, no new daemons or services |
| V. Minimal Trust Surface | Pass | Deep link/biometric/keystore bypasses gated behind debug builds |
| VI. Simplicity | Pass | No orchestration code — runner manages everything |

## Project Structure

Minimal footprint. The runner manages everything; only artifacts produced are:

```
specs/001-e2e-android-testing/
  spec.md                    # Feature spec (exists)
  research.md                # Architecture decisions (this file)
  plan.md                    # This plan
  tasks.md                   # Task list (generated next)
  learnings.md               # Cross-task discoveries (managed by runner)
validate/e2e/                # Runtime artifacts (gitignored)
  findings.json              # Bug findings with pass/fail per screen/flow
  screenshots/               # Screenshot evidence (PNG)
```

No new source files, scripts, or infrastructure code are created by this feature.

## Tool Environment Inventory

| Command | Tool | Nix package | Notes |
|---------|------|-------------|-------|
| `start-emulator` | Android emulator | `nix/android-emulator.nix` | In devshell, needs KVM |
| `make android-apk` | Gradle wrapper | `nix/android-apk.nix` | Builds debug APK |
| `adb install` | Android Debug Bridge | `androidsdk` | In devshell |
| `adb shell getprop` | Emulator readiness | `androidsdk` | Boot check |
| `adb -e emu finger touch` | Biometric simulation | `androidsdk` | Fingerprint bypass |
| `mcp-android` | MCP server | `nix-mcp-debugkit#mcp-android` | UI automation tools |
| `headscale` | Mesh coordinator | `pkgs.headscale` | In devshell |
| `tailscale` | Mesh node | `pkgs.tailscale` | In devshell |
| `nix-key daemon` | SSH agent | `nix/package.nix` | Host-side daemon |

## Test Plan Matrix

| SC | Test Tier | What Agent Does | Infrastructure |
|----|-----------|----------------|----------------|
| SC-001 | MCP exploration (Phase 2) | Navigate all 7 screens, Screenshot + DumpHierarchy each, compare to UI_FLOW.md | Emulator + MCP |
| SC-002 | MCP exploration (Phase 2) | Exercise every nav flow from flowchart, verify transitions | Emulator + MCP |
| SC-003 | MCP + headscale (Phase 3) | Trigger sign from host, observe dialog via MCP, tap Approve, verify SSH exits 0 | Emulator + headscale + daemon |
| SC-004 | MCP exploration (Phase 4) | Enter invalid inputs per Field Validation table, verify error messages | Emulator + MCP |
| SC-005 | MCP + adb (Phase 4) | Create state, `adb shell am force-stop`, restart, verify state via DumpHierarchy | Emulator + MCP |
| SC-006 | E2E loop (Phase 5) | Full explore → find bug → fix → rebuild → verify cycle | Emulator + MCP + source code |

## Phase Structure

### Phase 1: Infrastructure Prerequisites

Verify the runner's infrastructure works before giving agents MCP tools.

**Tasks**:
1. Verify emulator boots via `start-emulator` and MCP tools respond (Screenshot returns an image, DumpHierarchy returns XML)
2. Verify test bypass mechanisms work: deep link pairing (`adb shell am start -a android.intent.action.VIEW -d "nix-key://pair?payload=..."`), fingerprint simulation (`adb -e emu finger touch 1`), software keystore (APK built with debug flag)
3. Verify headscale/tailscale/daemon setup: headscale starts, host tailscale joins, daemon exposes SSH_AUTH_SOCK, pre-auth keys are generated

**Dependencies**: None (foundational)  
**Done when**: All three verifications pass on a clean environment.

---

### Phase 2: Screen and Flow Validation (US1 — P1) [needs: mcp-android, e2e-loop]

Agent explores all 7 screens and navigation flows, validates against UI_FLOW.md.

**Tasks**:
1. **Screen validation**: Agent visits each screen (TailscaleAuth, ServerList, Pairing, KeyManagement, KeyDetail, SignRequestDialog, Settings), takes Screenshot, runs DumpHierarchy, and verifies every layout element from UI_FLOW.md exists. Records pass/fail per screen in findings.json.
2. **Navigation flow validation**: Agent exercises every navigation edge from the flowchart — first-launch flow (TailscaleAuth → ServerList with back stack cleared), ServerList → Pairing → ServerList, ServerList → KeyManagement → KeyDetail → KeyManagement → ServerList, ServerList → Settings → ServerList. Verifies transitions work and back navigation is correct.

**Dependencies**: Phase 1  
**FR coverage**: FR-004, FR-005  
**SC coverage**: SC-001, SC-002  
**Done when**: Every screen and flow from UI_FLOW.md has a pass/fail entry in findings.json with screenshot evidence.

---

### Phase 3: Cross-System Sign Round-Trip (US2 — P1) [needs: mcp-android, e2e-loop]

Agent verifies the core product flow — SSH signing via phone.

**Tasks**:
1. **Sign approval round-trip**: Set up headscale mesh (reusing patterns from `test/e2e/android_e2e_test.sh`), pair emulator app with host daemon via deep link, trigger `ssh-keygen -Y sign` from host. Agent observes sign dialog via MCP Screenshot + WaitForElement, taps Approve, verifies biometric via fingerprint simulation, confirms SSH operation exits 0.
2. **Sign denial round-trip**: Same setup, trigger sign request, agent taps Deny. Verify SSH operation fails with SSH_AGENT_FAILURE.

**Dependencies**: Phase 2 (need working navigation to reach pairing and key screens)  
**FR coverage**: FR-006, FR-007, FR-012  
**SC coverage**: SC-003  
**Done when**: Sign approval returns a valid signature, sign denial returns SSH_AGENT_FAILURE. Both verified on live emulator.

---

### Phase 4: Error Paths + Persistence (US3, US4 — P2) [needs: mcp-android, e2e-loop]

Agent tests error handling and state persistence.

**Tasks**:
1. **Error path validation**: Agent enters every invalid input from the Field Validation Reference Table via MCP Type/SetText:
   - TailscaleAuth: "not-a-real-key" → expect "Invalid auth key format"
   - KeyDetail: 65-char name → expect "Name must be 1-64 characters..."
   - KeyDetail: duplicate name → expect "A key with this name already exists"
   - Settings: "invalid:endpoint:format" → expect "Invalid endpoint format (expected host:port)"
   - Pairing: malformed QR via deep link → expect "Invalid QR code" or "Not a nix-key pairing code"
   
   Agent verifies exact error message text via DumpHierarchy.

2. **Persistence validation**: Agent creates app state (pairs a host, creates a key), then kills app via `adb shell am force-stop com.nixkey`, restarts app, and verifies via DumpHierarchy that:
   - Host still appears in ServerList
   - Key still appears in KeyManagement
   - Key shows locked state (unlock resets per spec)

**Dependencies**: Phase 3 (need paired host and key from sign round-trip setup)  
**FR coverage**: FR-008, FR-011  
**SC coverage**: SC-004, SC-005  
**Done when**: All error messages match spec text exactly. State survives force-stop/restart.

---

### Phase 5: Explore-Fix-Verify Loop (US5 — P3) [needs: mcp-android, e2e-loop]

Full cycle: explore finds bugs, fix agent patches source, runner rebuilds APK, verify agent confirms fixes.

**Tasks**:
1. **End-to-end loop**: If any prior phase recorded bugs in findings.json, the runner triggers the fix-verify cycle:
   - Fix agent reads findings.json, modifies source code (Kotlin, Go, Nix, proto — any area)
   - Runner rebuilds APK (`make android-apk`) and reinstalls on emulator
   - Verify agent re-checks each bug via MCP tools
   - Loop until all bugs are resolved or max iterations (3) reached
   - Each bug status in findings.json updates: new → fixed → verified_fixed (or verified_broken for regressions)

**Dependencies**: Phases 2, 3, 4  
**FR coverage**: FR-009, FR-010  
**SC coverage**: SC-006  
**Done when**: findings.json shows zero open bugs, or max iterations reached with remaining bugs documented.

## Interface Contracts

| IC | Name | Producer | Consumer(s) | Specification |
|----|------|----------|-------------|---------------|
| IC-001 | findings.json | Explore agent (Phases 2-4) | Fix agent, Verify agent (Phase 5) | JSON at `validate/e2e/findings.json`. Schema: `{ findings: [{ id, screen, flow, description, expected, actual, screenshot, status }] }`. Status enum: `new`, `fixed`, `verified_fixed`, `verified_broken`. |
| IC-002 | Screenshots | Explore/Verify agents | findings.json references | PNGs at `validate/e2e/screenshots/<finding-id>.png`. Referenced by `screenshot` field in IC-001. |
| IC-003 | Headscale mesh config | Phase 1 infra setup | Phase 3 sign tests | Headscale at `localhost:18080`, namespace `nixkey-e2e`, pre-auth keys via `headscale preauthkeys create`. Patterns from `test/e2e/android_e2e_test.sh`. |

## Critical Path

**Day-1 user flow**: Boot emulator → MCP tools work → explore a screen → see findings.json output.

| Checkpoint | Phase | What it proves |
|------------|-------|---------------|
| Emulator boots, MCP Screenshot works | Phase 1 | Infrastructure is functional |
| All 7 screens visited with screenshots | Phase 2 | Agent can explore the live app |
| Sign request approved end-to-end | Phase 3 | Core product flow works through the mesh |
| Error messages match spec | Phase 4 | App validates inputs correctly |
| Bug found, fixed, verified | Phase 5 | Full loop works |

## Complexity Tracking

| Decision | Justification | Constitution |
|----------|--------------|--------------|
| No new code artifacts | Runner handles everything — zero added complexity | VI. Simplicity |

No complexity violations. This feature adds zero source code to the project.

---

### Phase 6: Post-Task Validation

Verify everything still builds, tests pass, lint is clean, and CI would be green. This runs AFTER the E2E exploration phases and catches any regressions introduced by the fix agent.

**Tasks**:
1. **Go build + test**: Run `make test` and `make build`. Fix any failures introduced by fix-agent source changes.
2. **Android build + test**: Run `make android-apk` and verify the APK installs and launches on emulator. Run `./gradlew testDebugUnitTest` in `android/`.
3. **Lint**: Run `make lint` (golangci-lint + nixfmt). Fix any new lint warnings.
4. **Security scan**: Run `make security-scan`. Verify no new findings introduced by fix-agent changes.
5. **Existing E2E gate**: Run `test/e2e/android_e2e_test.sh --skip-build` to verify the existing CI regression test still passes after any source changes.
6. **CI workflow verification**: If any workflow files were modified, verify CI steps locally before pushing. Push to remote, verify CI passes. Fix failures in a loop.

**Dependencies**: Phase 5 (or whatever phase the fix agent last modified source code)
**Done when**: `make validate` passes, existing E2E gate passes, CI is green.

---

### Phase 7: PR Preparation

Create PR with all changes, ensure branch protection requirements are met.

**Tasks**:
1. **Commit findings**: Commit `validate/e2e/findings.json` (if it should be tracked) or ensure it's properly gitignored.
2. **Update CLAUDE.md**: If any new commands or patterns emerged, add them to the project documentation.
3. **Create PR**: Push branch, create PR to develop with summary of E2E findings and any fixes applied.

**Dependencies**: Phase 6
**Done when**: PR created, CI green, ready for review.

## Key Constraints

1. **No orchestration code**: Tasks do NOT create shell scripts, prompt templates, scenario runners, or any orchestration code. Agents use MCP tools directly.
2. **Single emulator**: All phases are sequential. No parallel task execution for MCP tasks.
3. **Existing test untouched**: `test/e2e/android_e2e_test.sh` is not modified.
4. **Debug-only bypasses**: Deep link, biometric sim, and software keystore are only available in debug builds. Agents must use the debug APK.
5. **Side-by-side architecture**: Emulator and host processes run directly on the machine (not nested in a VM). Requires KVM.
