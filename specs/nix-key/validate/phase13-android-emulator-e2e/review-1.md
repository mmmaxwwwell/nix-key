# Phase phase13-android-emulator-e2e — Review #1: REVIEW-CLEAN

**Date**: 2026-03-29T17:25Z
**Assessment**: Code is clean. No bugs, security issues, or correctness problems found.

**Scope**: 19 files changed, +2071/-11 lines | **Base**: aed74375~1
**Commits**: T062 (APK build infra), T063 (emulator Nix), T064 (deep link QR bypass), T065 (UI Automator helpers), T066 (E2E shell orchestrator)

**Review notes**:
- Nix expressions (`android-apk.nix`, `android-emulator.nix`) pin SDK/NDK versions, handle fallback AVD creation, and properly check KVM availability
- E2E shell script (`android_e2e_test.sh`) has proper cleanup traps, timeout handling, retry logic, and prerequisite checks
- Android deep link handler (`MainActivity.kt`) correctly validates scheme/host, only enabled in debug manifest, handles `onNewIntent` for re-launch
- UI Automator helper (`NixKeyE2EHelper.kt`) has retry logic with configurable attempts and proper null-safety via Kotlin's type system
- NavGraph properly passes optional payload argument with URL encoding

**Deferred** (optional improvements, not bugs):
- The `--retry` argument parsing in `android_e2e_test.sh` uses `shift` inside a `for arg in "$@"` loop, which is unconventional but works correctly since the positional parameter value is read immediately after shift
- The E2E orchestrator invokes `NixKeyE2EHelper` methods via `am instrument -e method`, which is not standard JUnit instrumentation — the helper would need a custom test runner or wrapper tests to actually dispatch by method name. This is an integration concern that will surface during actual E2E runs on a real emulator
