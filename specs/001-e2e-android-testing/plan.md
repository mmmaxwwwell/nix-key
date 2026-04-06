# Implementation Plan: Comprehensive E2E Integration Testing

**Branch**: `001-e2e-android-testing` | **Date**: 2026-04-05 | **Spec**: [spec.md](spec.md)  
**Input**: Feature specification from `/specs/001-e2e-android-testing/spec.md`

## Summary

Replace the existing shallow E2E test suite with comprehensive agent-driven visual testing. Agents use MCP-android tools to navigate the real Android app on an emulator, validating every screen, navigation flow, state machine transition, and error path against the UI_FLOW.md specification. All tests use a real headscale mesh for full-fidelity Tailscale networking. CI runs tests as pass/fail assertions; a separate local-only explore-fix-verify loop enables agents to discover, fix, and verify bugs autonomously.

## Technical Context

**Language/Version**: Bash (orchestrator), Go 1.22+ (host daemon/tools), Kotlin (Android app)  
**Primary Dependencies**: MCP-android (nix-mcp-debugkit), headscale, tailscale, Android SDK (API 34), gomobile  
**Storage**: Temp directories (`/tmp/nix-key-e2e.XXXXXX/`), `test-logs/e2e/` for output  
**Testing**: Agent-driven via MCP tools; structured JSON output aggregatable by `scripts/ci-summary.sh`  
**Target Platform**: Linux (CI runner with KVM), Android emulator (API 34, x86_64)  
**Project Type**: Test infrastructure (shell orchestrator + agent prompts + CI integration)  
**Performance Goals**: Full suite completes within 60 minutes on CI with KVM  
**Constraints**: Requires KVM for emulator; requires `nix develop` shell for tooling  
**Scale/Scope**: 7 screens, 5+ navigation flows, 4 state machines, ~25 test scenarios

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Nix-First | PASS | Emulator, headscale, tailscale, build tools all provided via Nix flake devshell. Android APK is the only non-Nix artifact (per constitution). |
| II. Security by Default | PASS | Tests validate mTLS, cert pinning, biometric policies, and security warning dialogs. No security relaxations — bypasses use existing debug mechanisms (deep link, software keystore fallback). |
| III. Test-First | PASS | This feature IS the test infrastructure. Tests are written against the spec before validating the app. |
| IV. Unix Philosophy | PASS | Orchestrator is a shell script. Components are separate processes. Output is structured JSON to stdout/files. |
| V. Minimal Trust Surface | PASS | Tests validate that key enumeration can be disabled, that each key has independent confirmation policy, and that mutual pairing confirmation works. |
| VI. Simplicity | PASS | Extends the existing `android_e2e_test.sh` orchestrator pattern rather than introducing a new framework. Uses existing infrastructure (headscale, emulator, MCP-android). |

No violations. No complexity tracking needed.

## Project Structure

### Documentation (this feature)

```text
specs/001-e2e-android-testing/
├── plan.md              # This file
├── spec.md              # Feature specification
├── research.md          # Phase 0 research decisions
├── data-model.md        # Test entities and state machines
├── quickstart.md        # How to run the tests
├── contracts/
│   ├── test-output.md   # Output format contract
│   └── e2e-runner.md    # Runner interface and MCP tool contract
└── checklists/
    └── requirements.md  # Spec quality checklist
```

### Source Code (repository root)

```text
test/e2e/
├── android_e2e_test.sh       # Extended orchestrator (CI + explore-fix-verify modes)
├── scenarios/                 # Test scenario definitions
│   ├── us1-screen-validation.sh    # US1: Agent-driven screen exploration
│   ├── us2-navigation-flows.sh     # US2: Navigation flow coverage
│   ├── us3-state-machines.sh       # US3: State machine transitions
│   ├── us4-error-paths.sh          # US4: Error path validation
│   ├── us6-cross-system.sh         # US6: Cross-system gRPC integration
│   └── us7-persistence.sh          # US7: Persistence and recovery
├── prompts/                   # Agent prompt templates
│   ├── explore-screen.md           # Prompt: explore and validate a screen
│   ├── explore-flow.md             # Prompt: exercise a navigation flow
│   ├── explore-state-machine.md    # Prompt: verify state machine transitions
│   ├── explore-error-path.md       # Prompt: test error handling
│   ├── fix-bugs.md                 # Prompt: batch-fix discovered bugs
│   ├── verify-fixes.md             # Prompt: verify bug fixes
│   └── supervisor-review.md        # Prompt: supervisor progress review
└── lib/
    ├── infrastructure.sh      # Headscale, tailscale, emulator, daemon setup/teardown
    ├── mcp-helpers.sh         # MCP-android server management
    ├── scenario-runner.sh     # Test scenario execution and result collection
    └── report.sh              # Structured JSON output generation

test-logs/e2e/                 # Output (gitignored)
├── summary.json
├── screenshots/
├── hierarchies/
├── bugs/                      # Local mode only
├── iterations/                # Local mode only
└── supervisor/                # Local mode only

scripts/
└── ci-summary.sh             # Updated to aggregate e2e results
```

**Structure Decision**: Extends the existing `test/e2e/` directory. Scenario scripts are modular (one per user story) and share infrastructure setup via `lib/`. Agent prompts are separate markdown files that reference `specs/nix-key/UI_FLOW.md` and `spec.md` as source of truth. The existing orchestrator is extended with new flags rather than replaced.

## Phase 1: Design Decisions

### Infrastructure Layer (`test/e2e/lib/infrastructure.sh`)

Extracted from the existing `android_e2e_test.sh` into a reusable library:

1. **Headscale setup**: localhost:18080, SQLite, self-signed EC P-256 TLS cert (domain: `headscale.test`), embedded DERP region 999, user `nixkey-e2e`
2. **Tailscale nodes**: Host + phone, pre-auth keys, isolated state directories
3. **Emulator management**: API 34, x86_64, KVM, swiftshader, 2GB RAM, Pixel 6 profile
4. **Daemon management**: nix-key daemon with SSH_AUTH_SOCK, control socket, XDG isolation
5. **MCP-android server**: Started after emulator boot, connected via ADB
6. **Cleanup**: Trap handler kills all PIDs, removes temp directory

### Agent Prompt Design

Each agent prompt follows this structure:
1. **Context**: Reference to spec sections (UI_FLOW.md screens, state machines, field validations)
2. **Available tools**: MCP-android tool list with usage patterns
3. **Task**: Specific exploration or verification objective
4. **Validation criteria**: Expected outcomes from acceptance scenarios
5. **Output format**: Structured result (pass/fail, screenshots, details)

Agents use Screenshot + DumpHierarchy for observation, Click/Type/Swipe for interaction, and WaitForElement for synchronization.

### Test Scenario Execution

Each scenario script:
1. Declares preconditions (infrastructure state required)
2. Invokes the agent with the appropriate prompt + spec context
3. Collects structured results (pass/fail, screenshots, timing)
4. Reports to the runner for aggregation

Scenarios are independent (given infrastructure is up) but run sequentially since they share a single Android emulator.

### CI Integration

The existing `.github/workflows/e2e.yml` is updated:
- Build step unchanged (gomobile AAR + Gradle)
- Test step calls `android_e2e_test.sh` with new scenario flags
- Retry logic preserved (3 attempts, 15min timeout each)
- Artifacts: `test-logs/e2e/` uploaded alongside existing artifacts
- `ci-summary.sh` updated to include E2E results in aggregate

### Explore-Fix-Verify Loop (Local Mode)

When `--explore-fix-verify` is passed:
1. **Explore phase**: Agents run all scenarios, collecting BugReports for failures
2. **Fix phase**: Fix agent receives BugReports, analyzes source code, applies batch fixes across any codebase area (Android, Go, Nix, proto)
3. **Rebuild phase**: `build-android-apk` rebuilds AAR + APK, reinstalls on emulator
4. **Verify phase**: Re-run failed scenarios to confirm fixes
5. **Supervisor phase**: Every N iterations, supervisor reviews progress and adjusts strategy
6. Loop repeats until all bugs are verified-fixed or max iterations reached

## Phase 2: Implementation Phases

### Phase A: Infrastructure Extraction (Foundation)

Extract shared infrastructure from `android_e2e_test.sh` into `test/e2e/lib/`:
- `infrastructure.sh`: headscale, tailscale, emulator, daemon setup/teardown
- `mcp-helpers.sh`: MCP-android server lifecycle
- `report.sh`: structured JSON output generation
- Update `android_e2e_test.sh` to use the extracted library

### Phase B: Agent Prompts and Scenario Framework

Write agent prompt templates in `test/e2e/prompts/`:
- Screen exploration prompt (references UI_FLOW.md per-screen details)
- Navigation flow prompt (references navigation flowchart)
- State machine prompt (references state machine diagrams)
- Error path prompt (references field validation table)

Build scenario runner (`test/e2e/lib/scenario-runner.sh`) that:
- Takes a scenario script + infrastructure state
- Invokes the agent with prompt + context
- Collects and formats results

### Phase C: P1 Test Scenarios (Screen Validation + Navigation Flows)

Implement scenarios for User Stories 1 and 2:
- `us1-screen-validation.sh`: Walk all 7 screens, verify elements against spec
- `us2-navigation-flows.sh`: Exercise all navigation paths (first launch, pairing, key management, sign request, settings)

### Phase D: P2 Test Scenarios (State Machines + Error Paths)

Implement scenarios for User Stories 3 and 4:
- `us3-state-machines.sh`: Key lifecycle, sign request lifecycle, Tailscale connection, pairing session transitions
- `us4-error-paths.sh`: Invalid auth keys, malformed QR, biometric failures, network timeouts

### Phase E: Explore-Fix-Verify Loop

Implement the local-only explore-fix-verify mode:
- Fix agent prompt (`fix-bugs.md`)
- Verify agent prompt (`verify-fixes.md`)
- Supervisor prompt (`supervisor-review.md`)
- Loop orchestration in `android_e2e_test.sh --explore-fix-verify`

### Phase F: P3 Test Scenarios (Cross-System + Persistence)

Implement scenarios for User Stories 6 and 7:
- `us6-cross-system.sh`: Full gRPC round-trip through headscale mesh
- `us7-persistence.sh`: App restart, process kill recovery, stale auth handling

### Phase G: CI Integration

Update CI pipeline:
- Modify `.github/workflows/e2e.yml` to use new scenario-based runner
- Update `scripts/ci-summary.sh` to aggregate E2E results
- Verify artifact uploads include `test-logs/e2e/`
