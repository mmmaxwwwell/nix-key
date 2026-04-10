# Interview Notes — 003-comprehensive-e2e

**Date**: 2026-04-09
**Preset**: local
**Nix available**: yes

## Context

This feature replaces and supersedes 001-e2e-android-testing. The app has changed significantly since 001 ran:
- 002-deficiency-fixes replaced the TailscaleBackend stub with real `tsnet`, added loading states to 3 screens, fixed Nix device loading, added STATUS column to CLI, fixed shutdown logging, added struct tag validation, fixed release verification script
- 001's findings were against a partially-broken app (stub Tailscale, missing loading states, broken device merge). Those findings may no longer be valid — bugs may be fixed or new bugs may have been introduced

This feature does a complete re-exploration of the entire app with MCP tools, starting fresh, then generates deterministic CI regression tests from every finding.

## Architecture Decision

**Clean-slate approach**: Don't reference 001-e2e findings. Re-explore everything from scratch because the app is materially different. The 001-e2e `findings.json` is historical — the fixes from 001 are in the codebase, but we need to verify they still hold and find new issues.

**Two outputs from every MCP exploration task**:
1. `findings.json` — the exploration record (bugs found, passed checks)
2. UI Automator regression tests — generated in the SAME task, not a deferred batch step. Each explore agent writes its own regression test for each bug it fixes, before marking the task done. This prevents the "12 unprotected fixes" problem from 001.

**Expanded scope beyond 001**:
- All 7 screens + all navigation flows (same as 001)
- Key export (clipboard, share, QR) — not in 001
- Security warning dialogs (auto-approve, none-unlock) — not in 001
- ECDSA-P256 key creation — not in 001
- Lock/unlock button + long-press re-lock — not in 001
- Connection state transitions — not in 001
- Multiple paired hosts — not in 001
- Concurrent sign requests — not in 001
- Sign timeout — not in 001
- Device revocation end-to-end — not in 001
- CLI command exercise — not in 001
- OTEL distributed tracing — not in 001
- Daemon restart resilience — not in 001
- Phone app restart resilience — not in 001

## Key Decisions

- Stay on `develop` branch
- Reuse `test/e2e/setup.sh` and `test/e2e/teardown.sh` infrastructure
- Reuse `NixKeyE2EHelper.kt` as the test helper base
- One MCP task per screen/flow (fresh context per screen)
- **Regression tests generated inline** — not deferred to a batch step
- Expanded scripted E2E test for CI scenarios that don't need MCP
- OTEL tracing requires Jaeger in setup.sh
- Second paired host requires second pre-auth key in headscale

## Dependencies

- 002-deficiency-fixes MUST be complete before this runs
- Emulator bootable, gomobile AAR real (not stub)
- `nix-mcp-debugkit` available as flake input
