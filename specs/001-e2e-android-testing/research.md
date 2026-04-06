# Research: E2E Android Testing

**Feature**: 001-e2e-android-testing  
**Created**: 2026-04-06  
**Preset**: public

## Decision 1: Use the parallel runner's built-in MCP E2E loop

**Decision**: All E2E testing uses the runner's `[needs: mcp-android, e2e-loop]` annotation. No custom orchestration code.

**Rationale**: The first attempt produced ~6000 lines of shell script orchestration (prompt templates, scenario runners, agent management harnesses) that duplicated what the runner already provides. The runner handles: emulator boot, APK build+install, MCP server lifecycle, explore-fix-verify loop coordination, and supervisor checks. Writing orchestration code is explicitly a non-goal in the spec.

**Alternatives rejected**:
- Custom shell orchestration (android_e2e_test.sh style): Already failed. Produces unmaintainable meta-framework that no agent can debug.
- Kotlin instrumented tests (UI Automator): Good for CI regression gates (the existing android_e2e_test.sh fills this role), but cannot adapt to UI changes or discover unexpected bugs. MCP exploration is complementary.

## Decision 2: Real headscale mesh, no mocks

**Decision**: Cross-system tests (US2) use a real headscale instance with Tailscale nodes for both the host daemon and the Android emulator. Reuse infrastructure patterns from `test/e2e/android_e2e_test.sh`.

**Rationale**: Constitution II (Security by Default) and III (Test-First) require testing the actual mTLS + Tailscale communication path. Mocking the mesh would hide the exact class of bugs this feature exists to find (cert pinning failures, Tailscale routing issues, gRPC over mTLS).

**Alternatives rejected**:
- Mock gRPC server on localhost: Defeats the purpose — the mesh IS the product.
- NixOS VM test with nested emulator: Requires nested KVM, unavailable on CI runners. Side-by-side architecture is correct per `reference/e2e-runtime.md`.

## Decision 3: Test bypass mechanisms already exist

**Decision**: Use three existing bypass mechanisms rather than building new ones:
1. **Deep link for QR scanning**: `nix-key://pair?payload=<base64>` — bypasses camera (debug builds only)
2. **Emulator fingerprint simulation**: `adb -e emu finger touch 1` — bypasses real biometric sensor
3. **Software keystore fallback**: `setIsStrongBoxBacked(false)` — bypasses hardware TEE

**Rationale**: All three are already implemented in the debug build. No new code needed. Constitution V (Minimal Trust Surface) is maintained because bypasses are debug-only.

**Alternatives rejected**:
- Building new test-only DI modules: Unnecessary — the bypasses already exist.
- Skipping hardware-dependent tests entirely: Would leave the core sign flow untested on emulator.

## Decision 4: Runner-managed task lifecycle via annotations

**Decision**: Tasks use `[needs: mcp-android, e2e-loop]` annotations. The runner handles:
- Emulator boot + readiness check (`adb shell getprop sys.boot_completed`)
- APK build (`make android-apk`) + install (`adb install`)
- MCP-android server start + connection
- Explore/fix/verify agent spawning and coordination
- Screenshot capture and findings.json management

**Rationale**: This is the entire point of the runner integration. Agents receive MCP tools and use them to interact with the live app. They do not write scripts, prompt templates, or orchestration code.

**Alternatives rejected**:
- Manual infrastructure setup per task: Duplicates runner capability, causes drift.
- Shared setup script sourced by tasks: This IS the anti-pattern from attempt 1.

## Decision 5: Existing android_e2e_test.sh unchanged

**Decision**: `test/e2e/android_e2e_test.sh` (839 lines) continues as the deterministic CI regression test. It is NOT modified, replaced, or refactored.

**Rationale**: The existing script is a working, deterministic E2E gate that runs in CI. MCP-driven exploration is complementary — it finds new bugs that scripted tests don't cover. Both approaches coexist. Per spec non-goals: "Replacing or refactoring the existing test/e2e/android_e2e_test.sh — it continues to work as-is for CI."

**Alternatives rejected**:
- Replacing android_e2e_test.sh with MCP exploration: MCP exploration is non-deterministic and slower. CI needs a deterministic gate.
- Merging both approaches: They serve different purposes and shouldn't be coupled.
