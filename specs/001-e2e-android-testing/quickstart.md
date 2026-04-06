# Quickstart: E2E Integration Testing

**Feature**: 001-e2e-android-testing

## Prerequisites

```bash
nix develop              # Enter devshell (provides headscale, tailscale, adb, emulator tools)
```

Ensure `/dev/kvm` is available and writable (required for emulator performance).

## Run E2E Tests (CI Mode)

```bash
# Full run: build APK, boot emulator, run all tests
./test/e2e/android_e2e_test.sh

# Skip APK build (use pre-built)
./test/e2e/android_e2e_test.sh --skip-build

# Run only P1 scenarios
./test/e2e/android_e2e_test.sh --priority=P1

# Run specific scenario pattern
./test/e2e/android_e2e_test.sh --scenarios="US1-*,US2-*"
```

## Run Explore-Fix-Verify Loop (Local Only)

```bash
# Start the agent-driven explore-fix-verify loop
./test/e2e/android_e2e_test.sh --explore-fix-verify

# Limit iterations
./test/e2e/android_e2e_test.sh --explore-fix-verify --max-iterations=10
```

## View Results

```bash
# Test results
cat test-logs/e2e/summary.json | jq .summary

# View failure screenshots
ls test-logs/e2e/screenshots/

# Aggregate with other test results
./scripts/ci-summary.sh
cat test-logs/ci/ci-summary.json | jq .
```

## Infrastructure

The test runner automatically manages:
- Android emulator (API 34, x86_64, KVM)
- Headscale coordination server (localhost:18080)
- Host + phone Tailscale nodes
- nix-key daemon (SSH agent)
- MCP-android server (emulator interaction)

All processes are cleaned up on exit via trap handler.
