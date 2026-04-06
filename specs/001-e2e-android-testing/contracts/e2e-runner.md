# Contract: E2E Test Runner Interface

**Feature**: 001-e2e-android-testing  
**Date**: 2026-04-05

## CLI Interface

The E2E test runner is invoked as a shell script, extending the existing `test/e2e/android_e2e_test.sh` pattern.

### CI Mode (default)

```bash
# Run all E2E tests as pass/fail assertions
./test/e2e/android_e2e_test.sh [options]

# Options (extending existing flags):
#   --skip-build        Skip APK build (use pre-built)
#   --retry=N           Number of retry attempts (default: 2)
#   --timeout=SECONDS   Per-test timeout (default: 900)
#   --scenarios=PATTERN Run only matching scenario IDs (e.g., "US1-*")
#   --priority=P1,P2    Run only scenarios at given priorities
```

**Exit codes**:
- 0: All scenarios passed
- 1: One or more scenarios failed
- 2: Infrastructure setup failed (emulator, headscale, etc.)

### Local Explore-Fix-Verify Mode

```bash
# Run the explore-fix-verify loop (local development only)
./test/e2e/android_e2e_test.sh --explore-fix-verify [options]

# Additional options for explore-fix-verify:
#   --max-iterations=N     Maximum explore-fix-verify cycles (default: 20)
#   --supervisor-interval=N  Supervisor reviews every N iterations (default: 10)
```

**Exit codes**:
- 0: All bugs found and verified fixed
- 1: Bugs remain unfixed after max iterations
- 2: Infrastructure failure

## Infrastructure Setup Contract

The runner sets up infrastructure in this order, matching the existing orchestrator pattern:

```
1. Create temp directory: /tmp/nix-key-e2e.XXXXXX/
2. Start headscale (localhost:18080, SQLite, self-signed TLS, DERP region 999)
3. Create headscale user "nixkey-e2e"
4. Generate pre-auth keys (host + phone)
5. Start host tailscaled, join headscale
6. Boot Android emulator (API 34, KVM, swiftshader)
7. Build + install debug APK (unless --skip-build)
8. Start nix-key daemon (SSH_AUTH_SOCK, control socket)
9. Start MCP-android server (connects to emulator via ADB)
10. Inject Tailscale auth key into app via MCP tools
11. Run pairing flow via deep link
12. Execute test scenarios
13. Cleanup: kill all processes, remove temp directory
```

## MCP-Android Tool Contract

Agents interact with the emulator exclusively through MCP-android tools:

| Tool | Purpose | Used For |
|------|---------|----------|
| Screenshot | Capture current screen as PNG | Visual verification, failure evidence |
| DumpHierarchy | Get UI element tree (XML) | Element discovery, state verification |
| Click | Tap at coordinates | Button presses, navigation |
| ClickBySelector | Tap element by resource-id/text/class | Targeted interactions |
| Swipe | Swipe gesture (direction + distance) | Scrolling, pull-to-refresh |
| Type | Input text via keyboard | Auth key entry, key name entry |
| SetText | Set text field value directly | Fast text input (bypasses keyboard) |
| Press | Press hardware/soft key (Back, Home, Enter) | Navigation, dialog dismissal |
| WaitForElement | Wait for element to appear (with timeout) | Synchronization, loading states |
| GetScreenInfo | Get screen dimensions and density | Layout calculations |

## Agent-Spec Reference Contract

Agents receive these specification artifacts as context for validation:

| Artifact | Path | Purpose |
|----------|------|---------|
| UI_FLOW.md | `specs/nix-key/UI_FLOW.md` | Screen layouts, navigation flowchart, state machines, field validations |
| spec.md | `specs/nix-key/spec.md` | FR numbers, acceptance criteria, security requirements |
| Field Validation Table | Embedded in UI_FLOW.md | Expected validation messages per field per screen |
| Navigation Flowchart | Embedded in UI_FLOW.md | Expected screen transitions |
| State Machines | Embedded in UI_FLOW.md | Key lifecycle, sign request, Tailscale, pairing states |
