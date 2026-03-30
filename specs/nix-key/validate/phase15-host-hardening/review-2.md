# Phase phase15-host-hardening — Review #2: REVIEW-FIXES

**Date**: 2026-03-30T14:47Z
**Fixes applied**:
- `.github/workflows/ci.yml:622`, `.github/workflows/fuzz.yml:67`: Fuzz crash corpus upload used `path: testdata/fuzz/` (repo root) but Go writes crash reproducers to per-package `testdata/fuzz/` subdirectories. Changed to `**/testdata/fuzz/` glob so artifacts are actually captured. Without this fix, fuzz crashes were silently lost in CI. (P1, confidence 95%). Commit: 865803a
- `scripts/security-scan.sh:83-87`: Semgrep error handler had a no-op body (`:`) when exit code >= 2 (internal tool error). This meant a completely broken semgrep would produce a green security report. Changed to `all_pass=false` so tool failures are correctly flagged. (P1, confidence 95%). Commit: 865803a

**Deferred** (optional improvements, not bugs):
- `.github/workflows/ci.yml:136`: Benchmark regression awk checks `$5` (new timing value) instead of the ratio/percentage column, causing false positives on any benchmark >= 4µs. Mitigated by `continue-on-error: true` so it cannot block CI.
- `.github/workflows/ci.yml:10-12`: `security-events: write` is workflow-scoped but only needed by the `security` job. All other jobs inherit unnecessary write permission.
- `nix/tests/adversarial-test.nix:363`: Multi-line JSON embedded in single-quoted shell `printf` is fragile if data ever contains single quotes (currently safe with known inputs).
- `nix/tests/adversarial-test.nix:368-374`: `find /nix/store` for config path could return empty string; `ln -sf ""` would silently create a broken symlink (currently works because the config file exists).
- Previously deferred items from review #1 remain unchanged (test fixture serial numbers, double Stop() panic risk, token replay assertion, store path leak assertion, security-scan.sh missing set -e)
