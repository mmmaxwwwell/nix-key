# Plan: 003-comprehensive-e2e

## Overview

Clean-slate E2E testing of the entire app after 002-deficiency-fixes. Every screen explored via MCP, every cross-system flow verified, every CLI command exercised, every bug fix protected by a regression test written inline. Supersedes 001-e2e-android-testing.

## Key Design Decision: Inline Regression Tests

001-e2e deferred test generation to a batch step that was never implemented — 12 bugs got fixed but only 1 got a test. This feature requires each MCP explore agent to write a UI Automator regression test for each bug it fixes, in the same task. The test is part of the "Done when" criteria. No fix is done until its test exists and passes.

## Phase Structure

### Phase 1: Infrastructure Verification
Verify emulator, APK, MCP tools, headscale mesh, second host, Jaeger collector.

### Phase 2: Screen Exploration (MCP — one task per screen)
7 tasks, one per screen from UI_FLOW.md. Each agent explores its screen, compares against the spec, fixes bugs, writes regression tests. Includes new loading states from 002.

### Phase 3: Flow Validation (MCP)
Navigation flows, error paths, persistence across restart.

### Phase 4: Extended Function Exploration (MCP)
Key export, security warnings, lock/unlock, multi-host, ECDSA-P256, connection state transitions.

### Phase 5: Cross-System Tests (Scripted)
Sign approve/deny/timeout/concurrent, multi-key, revocation, CLI exercise. These are deterministic and go in `test/e2e/android_e2e_test.sh`.

### Phase 6: Resilience + OTEL (Scripted)
Daemon restart, app restart, network partition, OTEL trace verification.

### Phase 7: Final Validation
Full test suite, all regression tests, expanded E2E script.

## Testing Strategy

| Phase | Method | Output |
|-------|--------|--------|
| 2-4 | MCP explore→fix→verify | findings.json + inline regression tests |
| 5-6 | Shell script orchestrated | Pass/fail per scenario in test output |
| 7 | Full suite | All green |

## Interface Contracts

| IC | Name | Producer | Consumer | Format |
|----|------|----------|----------|--------|
| IC-F01 | findings.json | MCP explore agents | Human review, future audits | `{findings: [{id, status, title, screen, severity}]}` |
| IC-F02 | Regression tests | MCP explore agents | `./gradlew connectedDebugAndroidTest` | Kotlin test classes in `e2e/regression/` using `NixKeyE2EHelper` |
| IC-F03 | E2E script scenarios | Scripted test tasks | `e2e.yml` CI workflow | Exit 0 from `test/e2e/android_e2e_test.sh` |
