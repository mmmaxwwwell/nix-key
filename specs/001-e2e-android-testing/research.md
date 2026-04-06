# Research: Comprehensive E2E Integration Testing

**Feature**: 001-e2e-android-testing  
**Date**: 2026-04-05

## Decision 1: Test Architecture — Side-by-Side vs NixOS VM

**Decision**: Side-by-side architecture (all components run natively on the host/CI runner, not inside NixOS VMs).

**Rationale**: The existing E2E orchestrator (`test/e2e/android_e2e_test.sh`) already uses this pattern successfully — headscale, tailscaled, nix-key daemon, and the Android emulator all run as native processes on the CI runner. The Android emulator requires KVM access which is straightforward on a native runner but complex inside a NixOS VM. The existing CI workflow (`.github/workflows/e2e.yml`) already enables KVM via udev rules on `ubuntu-latest`.

**Alternatives considered**:
- NixOS VM wrapping everything: rejected because the Android emulator needs KVM and nested virtualization adds complexity and latency without benefit.
- Hybrid (NixOS VM for host services, native emulator): rejected because it complicates networking between VM and emulator; the existing side-by-side pattern already works.

## Decision 2: MCP-Android Integration Point

**Decision**: The MCP-android server from `nix-mcp-debugkit` runs as a sidecar process alongside the emulator. Agents connect to it via MCP protocol (stdio or SSE) and use it to drive the emulator UI. The existing `NixKeyE2EHelper.kt` UI Automator code serves as reference for expected UI element selectors and interaction patterns but is not reused directly — agents use MCP tools instead.

**Rationale**: MCP-android wraps `adb` and `uiautomator2` behind a tool interface that agents can use natively. The existing E2E helper's retry logic and element selectors (e.g., `waitForApp()`, `navigateToKeys()`, `createKey()`) document the app's UI contract and inform the agent prompts. The MCP tools (Screenshot, DumpHierarchy, Click, ClickBySelector, Swipe, Type, SetText, Press, WaitForElement, GetScreenInfo) map directly to the operations the existing helper performs.

**Alternatives considered**:
- Reuse NixKeyE2EHelper.kt directly via instrumented tests: rejected because it's shallow (mocked ViewModels) and doesn't support the agent-driven explore pattern.
- Write new UI Automator tests: rejected because agents need visual feedback loops (screenshot → reason → act) that UI Automator alone doesn't support.

## Decision 3: Headscale Setup for All Tests

**Decision**: All E2E tests use a real headscale mesh, following the same pattern as the existing orchestrator. Headscale runs on localhost:18080 with SQLite, self-signed TLS cert, embedded DERP (region 999). Pre-authorized auth keys are generated programmatically for both the host tailscaled and the phone (emulator).

**Rationale**: Clarification from user — all tests use real headscale, no mocks. The existing orchestrator already implements this pattern with namespace `nixkey-e2e`, pre-auth keys, and XDG directory isolation. Setup adds ~30-60s which is negligible in a 60-minute budget.

**Alternatives considered**:
- Mock Tailscale state for P1/P2: rejected per clarification — avoids maintaining a mock layer and provides higher fidelity.

## Decision 4: Test Bypass Mechanisms

**Decision**: Use the following bypass mechanisms, all of which already exist in the codebase:

| Bypass | Mechanism | Source |
|--------|-----------|--------|
| QR code scanning | Deep link: `nix-key://pair?payload=<base64>` | `src/debug/AndroidManifest.xml` intent-filter, `MainActivity.extractPairPayload()` |
| Hardware keystore | Ed25519 uses BouncyCastle software + AES wrapping key; ECDSA falls back from StrongBox to TEE | `KeyManager.kt` |
| Tailscale auth | Pre-authorized auth key injected via UI Automator or `adb input text` | `android_e2e_test.sh` step 4 |
| Biometric auth | Android emulator test biometric enrollment via `adb` fingerprint simulation | Standard emulator API |

**Rationale**: All bypasses are already implemented and tested in the existing E2E flow. No new debug-only code paths needed.

**Alternatives considered**:
- Build-time feature flags for test mode: rejected because the existing mechanisms are sufficient and don't require maintaining separate build variants.

## Decision 5: Test Output Format

**Decision**: Structured JSON output compatible with the existing `cmd/test-reporter` format. Each test scenario produces a result entry with: test name, status (pass/fail/skip), duration, screenshots on failure, and error details. Output goes to `test-logs/e2e/` alongside existing test output directories.

**Rationale**: The project already has a structured test reporting pipeline (`cmd/test-reporter` reads `go test -json`, produces `test-logs/*/summary.json`). The E2E suite should produce output in the same schema so `scripts/ci-summary.sh` can aggregate it with other test results.

**Alternatives considered**:
- Custom report format: rejected because it would require modifying `ci-summary.sh` and the test reporter.
- JUnit XML: rejected because the existing pipeline uses JSON, not XML.

## Decision 6: Explore-Fix-Verify Loop Architecture

**Decision**: The loop is a local-only development tool using the spec-kit parallel runner with `[needs: mcp-android, e2e-loop]` task annotations. Fix agents can modify any source code (Android, Go, Nix, proto) that the bug traces to. The loop does not run in CI — CI runs the test suite as standard pass/fail assertions.

**Rationale**: Per clarification, the loop is for local development. It uses the spec-kit runner's explore-fix-verify pattern: explore agents find bugs, fix agents batch-fix them, verify agents confirm fixes. A supervisor reviews every 10 iterations. Rebuilds include both the Go gomobile AAR and the Android APK.

**Alternatives considered**:
- CI-integrated fix loop: rejected per clarification — too dangerous for automated environments.
- Android-only fixes: rejected per clarification — bugs may trace to Go, Nix, or proto code.

## Decision 7: Emulator Configuration

**Decision**: Use the existing `nix/android-emulator.nix` configuration: API 34, x86_64, Pixel 6 profile, 2GB RAM, swiftshader_indirect GPU, KVM auto-detection. Boot timeout: 120s with KVM, 600s without.

**Rationale**: The existing configuration is battle-tested in CI. No changes needed.

**Alternatives considered**:
- API 35: rejected because the current system image and SDK are pinned to API 34 with build tools 35.0.0, and changing would require updating the entire Android build chain.
