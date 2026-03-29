# Phase phase6-pairing-flow — Review #1: REVIEW-FIXES

**Date**: 2026-03-29T05:50:00Z
**Fixes applied**:
- `internal/pairing/server.go:211-214`: The pairing server's `handlePair` method never called `Shutdown` after consuming the one-time token. This caused `Serve()` to block indefinitely in `RunPair`, preventing post-pairing processing (cert encryption, device registration, daemon notification) from ever executing. A successful pairing would hang until the user hit Ctrl+C, at which point `RunPair` returned `ctx.Err()` (an error) instead of `nil`. Fix: added `go s.httpServer.Shutdown(context.Background())` at the end of `handlePair` after a valid token is consumed. Updated token replay tests to handle connection refusal (server shut down) as an acceptable rejection. Commit: c70b8ae.

**Deferred** (optional improvements, not bugs):
- `server.go:159-167`: Both branches of the `tokenWasUsed` check in `handlePair` return the same "denied" response — the conditional is cosmetic dead code. Not a bug.
- `server.go:184-187`: The confirm callback goroutine leaks if the timeout fires before the callback returns. Minor since the server shuts down shortly after.
