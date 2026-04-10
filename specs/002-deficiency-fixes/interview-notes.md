# Interview Notes — 002-deficiency-fixes

**Date**: 2026-04-09
**Preset**: local (scoped fix, no new infra needed)
**Nix available**: yes

## Context

This feature was generated from a systematic audit of all completed tasks (T001-T113) against their original task specifications. 7 tasks were found to have deficient implementations where the code does not satisfy the spec requirements.

## Deficiency Summary

| Task | Component | Issue |
|------|-----------|-------|
| T009 | `internal/config/` | No struct tag validation — only custom `validate()` function |
| T010 | `internal/daemon/shutdown.go` | Missing "log initiated" and "flush logs" steps |
| T034 | `TailscaleBackend` in `AppModule.kt` | Hardcoded stub returning fake IP, no real libtailscale |
| T044 | `internal/config/` + `cmd/nix-key/daemon.go` | Config has no Devices field; daemon passes nil to Merge() |
| T054 | `cmd/nix-key/devices.go` | Shows SOURCE column instead of STATUS column |
| T072 | `scripts/verify-release-pipeline.sh` | Incorrect grep patterns (push: vs workflow_run:) |
| T089 | Android UI screens | Loading states missing from 3 screens |

## Key Decisions

- Fix on `develop` branch directly — no feature branch needed for fixes
- All fixes are independent — can be implemented in parallel
- No new infrastructure or dependencies needed (except go-playground/validator for T009)
- Existing tests must continue to pass
- New tests required for each fix to verify the deficiency is resolved
