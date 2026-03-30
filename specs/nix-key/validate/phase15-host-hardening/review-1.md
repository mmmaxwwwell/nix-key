# Phase phase15-host-hardening — Review #1: REVIEW-FIXES

**Date**: 2026-03-30T14:30Z
**Fixes applied**:
- `.github/workflows/fuzz.yml`: Moved `github.event.inputs.fuzztime` from direct `${{ }}` interpolation in shell to `env:` block to prevent script injection via `workflow_dispatch` input (P1, confidence 95%). Commit: 84c2612
- `flake.nix`: Made `infer` package conditional on `system == "x86_64-linux"` since the package only supports that platform, preventing `nix develop` failure on macOS/aarch64 (P1, confidence 90%). Commit: 84c2612
- `pkg/phoneserver/bench_test.go`: Replaced reuse of 5-second timeout context in benchmark loop with `context.Background()` — the old context would expire during `b.N` auto-calibration causing `DeadlineExceeded` (P1, confidence 95%). Commit: 84c2612

**Deferred** (optional improvements, not bugs):
- `test/fixtures/gen/main.go:409`: Adversarial leaf certs share serial number 100 (RFC 5280 violation), but only affects test fixtures — no runtime impact
- `cmd/nix-key/hardening_test.go:523-588`: `TestIntegrationControlSocketRoundTrip` calls `srv.Stop()` explicitly (needed for testing shutdown behavior) but lacks `t.Cleanup` fallback — double-calling `Stop()` would panic on `close(s.done)`. Fix requires making `Stop()` idempotent (sync.Once), which is a code change beyond review scope
- `nix/tests/adversarial-test.nix:549`: Token replay test assertion on "000" in curl output could match body content; low risk since test bodies are small
- `nix/tests/adversarial-test.nix:600`: Nix store path leak assertion is overly broad (matches "nix-key" in command name); low risk since it's a defense-in-depth test
- `scripts/security-scan.sh`: Missing `set -e` could allow silent failures in summary generation
