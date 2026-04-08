# Phase phase2-screen-and-flow-vali — Review #1: REVIEW-FIXES

**Date**: 2026-04-08
**Fixes applied**:
- `internal/pairing/server.go:159`: Token comparison used `!=` (timing side-channel). Changed to `crypto/subtle.ConstantTimeCompare` for constant-time comparison. Commit: 8b5fe4a
- `internal/daemon/control.go:137`: `Stop()` called `close(s.done)` without guard; double-close panics. Added `sync.Once` wrapper. Commit: 8b5fe4a

**Deferred** (optional improvements, not bugs):
- `.github/workflows/ci.yml:489,506,515`: Snyk and SonarCloud actions pinned to `@master` (mutable branch). Should pin to version tag or commit SHA for supply chain safety.
- `internal/agent/backend.go:265-272`: SSH signature unmarshal fallback treats raw bytes as valid signature. Intentional design for format flexibility but could obscure phone-side bugs.
- `android/app/src/main/java/com/nixkey/ui/viewmodel/PairingViewModel.kt:216-219`: Certificate fingerprint computed from PEM text rather than DER bytes. Works consistently since both sides use deterministic PEM encoding, but DER would be more robust.
- `android/app/src/main/java/com/nixkey/keystore/KeyManager.kt:239-252`: Ed25519 private key bytes not zeroed after wrapping during creation (zeroed during signing). Minor on Android due to process isolation.
- `.golangci.yml:30`: SA1019 (deprecated API usage) suppressed in staticcheck. Review needed to ensure no deprecated crypto APIs are being used silently.
- `android/gradle/libs.versions.toml:68`: `mockk-android` used for JVM unit tests; should use `mockk` (without `-android`) for `testImplementation`.
