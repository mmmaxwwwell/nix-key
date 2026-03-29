# Phase phase7-android-core-keystor — Review #1: REVIEW-FIXES

**Date**: 2026-03-29T03:59:00Z
**Scope**: 56 files changed, +5841/-10 lines | Base: 3f3504afd8d1eee8cee89e7b805dc98150932ec3~1
**Commits**: T028-T035 (Android project setup, KeyManager, Compose UI, BiometricHelper, SignRequestDialog, gomobile bridge, TailscaleManager, GrpcServerService)

**Fixes applied**:
- `android/app/src/main/java/com/nixkey/keystore/SignRequestQueue.kt`: Race condition in `enqueue()`/`complete()`/`clear()` — concurrent enqueue calls could both see `_currentRequest.value == null`, both call `advanceQueue()`, and silently lose the first request (it gets polled but immediately overwritten). Fixed by adding `synchronized(lock)` around all mutating methods. Commit: 3096314.

**Deferred** (optional improvements, not bugs):
- `SignRequestQueue.queueSize` docstring says "including the one currently displayed" but the actual count excludes it. Behavior is correct; docstring is misleading.
- `KeyDetailScreen` delete button has no confirmation dialog — accidental deletion possible but recoverable (create new key).
- `SettingsRepository` uses plain `SharedPreferences` for non-sensitive settings (booleans, endpoint string) — acceptable, not a security issue.
