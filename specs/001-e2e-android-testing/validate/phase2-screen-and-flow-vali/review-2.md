# Phase phase2-screen-and-flow-vali — Review #2: REVIEW-FIXES

**Date**: 2026-04-08T18:02Z
**Fixes applied**:
- `android/app/src/main/java/com/nixkey/ui/viewmodel/ServerListViewModel.kt:46-48`: Connection timeout handler called `tailscaleManager.stop()` on the main thread (via `viewModelScope` default dispatcher). Since `stop()` invokes `backend.stop()` — a synchronous libtailscale binding that performs network I/O — this could cause ANR on real devices. Wrapped in `withContext(Dispatchers.IO)` to match the `retryConnection()` pattern. Commit: 89c2344

**Deferred** (optional improvements, not bugs):
- `android/gradle/libs.versions.toml:68`: `mockk-android` used for JVM unit tests; should use `mockk` (without `-android`) for `testImplementation` (carried from review #1).
- `.github/workflows/ci.yml:489,506,515`: Snyk and SonarCloud actions pinned to `@master` (mutable branch). Should pin to version tag or commit SHA for supply chain safety (carried from review #1).
