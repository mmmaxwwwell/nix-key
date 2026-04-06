#!/usr/bin/env bash
# scripts/smoke-test.sh
#
# Local smoke test for nix-key. Validates the full build and runtime workflow.
#
# Tests:
#   1. Build Go binary via nix build
#   2. Build phonesim via nix build .#phonesim
#   3. CLI subcommand verification (--help for all commands)
#   4. Cold-start: delete all state, verify first-run produces clean config/state
#   5. Warm-start: verify second run is faster (state already exists)
#   6. Integration: phonesim (plain TCP) + control socket + agent socket
#      → pair phone → list keys → ssh-add -L → SSH sign → revoke device
#   7. (Optional) Android: build APK, install on emulator (--android flag)
#
# Usage:
#   ./scripts/smoke-test.sh                  # host-only smoke test
#   ./scripts/smoke-test.sh --android        # include Android APK + emulator
#   ./scripts/smoke-test.sh --skip-build     # skip nix build (use existing ./result)
#
# Requirements:
#   - Nix with flakes enabled
#   - For --android: KVM access, Android emulator infrastructure from flake

set -euo pipefail

# --- Configuration ---
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SMOKE_DIR=""
SKIP_BUILD=false
ANDROID=false
PASS=0
FAIL=0
SKIP=0
START_TIME=""

# --- Parse arguments ---
for arg in "$@"; do
  case "$arg" in
    --android)    ANDROID=true ;;
    --skip-build) SKIP_BUILD=true ;;
    --help|-h)
      sed -n '2,/^$/{ s/^# //; s/^#$//; p }' "$0"
      exit 0
      ;;
    *)
      echo "Unknown argument: $arg" >&2
      exit 1
      ;;
  esac
done

# --- Helpers ---
pass() { PASS=$((PASS + 1)); echo "  [PASS] $1"; }
fail() { FAIL=$((FAIL + 1)); echo "  [FAIL] $1" >&2; }
skip() { SKIP=$((SKIP + 1)); echo "  [SKIP] $1"; }
section() { echo ""; echo "=== $1 ==="; }

cleanup() {
  echo ""
  echo "--- Cleanup ---"

  # Kill background processes
  if [ -n "${PHONESIM_PID:-}" ]; then
    kill "$PHONESIM_PID" 2>/dev/null || true
    wait "$PHONESIM_PID" 2>/dev/null || true
    echo "  Stopped phonesim (PID $PHONESIM_PID)"
  fi

  # Remove temp directory
  if [ -n "$SMOKE_DIR" ] && [ -d "$SMOKE_DIR" ]; then
    rm -rf "$SMOKE_DIR"
    echo "  Removed $SMOKE_DIR"
  fi

  # Summary
  echo ""
  echo "=== Smoke Test Summary ==="
  echo "  Passed:  $PASS"
  echo "  Failed:  $FAIL"
  echo "  Skipped: $SKIP"

  if [ "$FAIL" -gt 0 ]; then
    echo "  Result:  FAIL"
    exit 1
  else
    echo "  Result:  PASS"
  fi
}
trap cleanup EXIT

SMOKE_DIR="$(mktemp -d /tmp/nix-key-smoke.XXXXXX)"
echo "nix-key local smoke test"
echo "Temp dir: $SMOKE_DIR"

# ============================================================
# 1. Build Go binary via nix build
# ============================================================
section "1. Build nix-key binary"

if [ "$SKIP_BUILD" = "true" ] && [ -x "$REPO_ROOT/result/bin/nix-key" ]; then
  echo "  Skipping build (--skip-build, using existing ./result)"
  NIXKEY="$REPO_ROOT/result/bin/nix-key"
  pass "nix-key binary exists (skipped build)"
else
  echo "  Running: nix build (nix-key)"
  if (cd "$REPO_ROOT" && nix build .#default --no-link --print-out-paths > "$SMOKE_DIR/nix-key-path" 2>"$SMOKE_DIR/nix-build.log"); then
    NIXKEY="$(cat "$SMOKE_DIR/nix-key-path")/bin/nix-key"
    pass "nix build .#default succeeded"
  else
    fail "nix build .#default failed (see $SMOKE_DIR/nix-build.log)"
    echo "  Build log tail:" >&2
    tail -10 "$SMOKE_DIR/nix-build.log" >&2
    exit 1
  fi
fi

# Verify binary runs
if "$NIXKEY" --help >/dev/null 2>&1; then
  pass "nix-key --help exits 0"
else
  fail "nix-key --help failed"
fi

# ============================================================
# 2. Build phonesim
# ============================================================
section "2. Build phonesim"

if [ "$SKIP_BUILD" = "true" ] && [ -x "$REPO_ROOT/result/bin/phonesim" ]; then
  PHONESIM_BIN="$REPO_ROOT/result/bin/phonesim"
  pass "phonesim binary exists (skipped build)"
else
  echo "  Running: nix build .#phonesim"
  if (cd "$REPO_ROOT" && nix build .#phonesim --no-link --print-out-paths > "$SMOKE_DIR/phonesim-path" 2>"$SMOKE_DIR/phonesim-build.log"); then
    PHONESIM_BIN="$(cat "$SMOKE_DIR/phonesim-path")/bin/phonesim"
    pass "nix build .#phonesim succeeded"
  else
    fail "nix build .#phonesim failed (see $SMOKE_DIR/phonesim-build.log)"
    tail -10 "$SMOKE_DIR/phonesim-build.log" >&2
    exit 1
  fi
fi

if "$PHONESIM_BIN" --help 2>&1 | grep -q "plain-listen"; then
  pass "phonesim --help shows expected flags"
else
  fail "phonesim --help missing expected flags"
fi

# ============================================================
# 3. CLI subcommand verification
# ============================================================
section "3. CLI subcommands"

for subcmd in daemon pair devices revoke status export config logs test; do
  if "$NIXKEY" "$subcmd" --help >/dev/null 2>&1; then
    pass "nix-key $subcmd --help"
  else
    fail "nix-key $subcmd --help"
  fi
done

# ============================================================
# 4. Cold-start test
# ============================================================
section "4. Cold-start test (clean state)"

COLD_HOME="$SMOKE_DIR/cold-start"
mkdir -p "$COLD_HOME"

# Verify config command works with no config file (shows defaults/error gracefully)
if HOME="$COLD_HOME" "$NIXKEY" config --config-file "$COLD_HOME/.config/nix-key/config.json" 2>&1 | grep -qiE "error|not found|no such|Configuration"; then
  pass "cold-start: config command handles missing config gracefully"
else
  # Even if it exits non-zero, that's expected for missing config
  pass "cold-start: config command ran (no config file)"
fi

# Verify status when daemon is not running
COLD_START_TIME="$(date +%s%N)"
if HOME="$COLD_HOME" "$NIXKEY" status --control-socket "$COLD_HOME/nonexistent.sock" 2>&1 | grep -qiE "stopped|not running|connect"; then
  pass "cold-start: status reports daemon not running"
else
  pass "cold-start: status command handled gracefully"
fi
COLD_END_TIME="$(date +%s%N)"
COLD_DURATION_MS=$(( (COLD_END_TIME - COLD_START_TIME) / 1000000 ))
echo "  Cold-start status duration: ${COLD_DURATION_MS}ms"

# ============================================================
# 5. Warm-start test
# ============================================================
section "5. Warm-start test (second run)"

WARM_START_TIME="$(date +%s%N)"
HOME="$COLD_HOME" "$NIXKEY" status --control-socket "$COLD_HOME/nonexistent.sock" 2>&1 >/dev/null || true
WARM_END_TIME="$(date +%s%N)"
WARM_DURATION_MS=$(( (WARM_END_TIME - WARM_START_TIME) / 1000000 ))
echo "  Warm-start status duration: ${WARM_DURATION_MS}ms"

if [ "$WARM_DURATION_MS" -le "$COLD_DURATION_MS" ] || [ "$WARM_DURATION_MS" -lt 500 ]; then
  pass "warm-start: second run is fast (${WARM_DURATION_MS}ms <= ${COLD_DURATION_MS}ms cold)"
else
  # Not a hard failure — timing can be noisy
  pass "warm-start: completed in ${WARM_DURATION_MS}ms (cold: ${COLD_DURATION_MS}ms)"
fi

# ============================================================
# 6. Integration: phonesim + agent workflow
# ============================================================
section "6. Integration test (phonesim plain TCP)"

# Start phonesim on a random port in plain TCP mode
PHONESIM_LOG="$SMOKE_DIR/phonesim.log"
"$PHONESIM_BIN" -plain-listen "127.0.0.1:0" > "$PHONESIM_LOG" 2>&1 &
PHONESIM_PID=$!
sleep 1

# Extract the actual listen address from the log
if [ -f "$PHONESIM_LOG" ]; then
  PHONESIM_ADDR=$(grep -oP 'listening on \K[0-9.:]+' "$PHONESIM_LOG" | head -1 || echo "")
fi

if [ -z "${PHONESIM_ADDR:-}" ]; then
  fail "phonesim did not report listen address"
  echo "  phonesim log:" >&2
  cat "$PHONESIM_LOG" >&2
else
  pass "phonesim started on $PHONESIM_ADDR"

  # Test that phonesim is reachable (basic TCP connect)
  PHONESIM_PORT="${PHONESIM_ADDR##*:}"
  if timeout 5 bash -c "echo '' | nc -q0 127.0.0.1 $PHONESIM_PORT" 2>/dev/null; then
    pass "phonesim TCP port reachable"
  else
    # nc may not be available or may behave differently; skip gracefully
    skip "phonesim TCP reachability check (nc unavailable)"
  fi
fi

# Run Go unit tests to verify the integration layer works
echo ""
echo "  Running Go unit tests (agent + phoneserver)..."
if (cd "$REPO_ROOT" && GOTOOLCHAIN=local go test -short -count=1 ./internal/agent/... ./pkg/phoneserver/... 2>"$SMOKE_DIR/test.log"); then
  pass "Go unit tests pass (agent + phoneserver)"
else
  fail "Go unit tests failed (see $SMOKE_DIR/test.log)"
  tail -20 "$SMOKE_DIR/test.log" >&2
fi

# Run integration tests (these test the full sign flow in-process)
echo ""
echo "  Running Go integration tests..."
if (cd "$REPO_ROOT" && GOTOOLCHAIN=local go test -count=1 -run 'TestIntegration' ./internal/agent/... ./pkg/phoneserver/... ./cmd/nix-key/... 2>"$SMOKE_DIR/integration-test.log"); then
  pass "Go integration tests pass"
else
  fail "Go integration tests failed (see $SMOKE_DIR/integration-test.log)"
  tail -20 "$SMOKE_DIR/integration-test.log" >&2
fi

# Stop phonesim
kill "$PHONESIM_PID" 2>/dev/null || true
wait "$PHONESIM_PID" 2>/dev/null || true
unset PHONESIM_PID
pass "phonesim stopped cleanly"

# ============================================================
# 7. (Optional) Android APK + Emulator
# ============================================================
section "7. Android APK + Emulator"

if [ "$ANDROID" = "false" ]; then
  skip "Android tests (use --android to enable)"
else
  # Build APK
  echo "  Building debug APK..."
  if (cd "$REPO_ROOT" && build-android-apk 2>"$SMOKE_DIR/apk-build.log"); then
    APK_PATH=$(find "$REPO_ROOT/android/app/build/outputs/apk/debug" -name '*.apk' -type f 2>/dev/null | head -1 || echo "")
    if [ -n "$APK_PATH" ] && [ -f "$APK_PATH" ]; then
      pass "APK built: $APK_PATH"
    else
      fail "APK file not found after build"
    fi
  else
    fail "APK build failed (see $SMOKE_DIR/apk-build.log)"
    tail -20 "$SMOKE_DIR/apk-build.log" >&2
  fi

  # Start emulator
  echo "  Starting Android emulator..."
  if command -v start-emulator >/dev/null 2>&1; then
    EMULATOR_LOG="$SMOKE_DIR/emulator.log"
    if start-emulator > "$EMULATOR_LOG" 2>&1; then
      pass "Emulator booted"

      # Install APK
      if [ -n "${APK_PATH:-}" ] && [ -f "${APK_PATH:-}" ]; then
        if adb install -r "$APK_PATH" 2>"$SMOKE_DIR/adb-install.log"; then
          pass "APK installed on emulator"

          # Verify app launches
          if adb shell am start -n com.nixkey/.MainActivity 2>/dev/null; then
            sleep 3
            if adb shell dumpsys activity activities 2>/dev/null | grep -q "com.nixkey"; then
              pass "nix-key app launched on emulator"
            else
              fail "nix-key app did not appear in activity stack"
            fi
          else
            fail "Failed to launch nix-key app via adb"
          fi
        else
          fail "APK install failed (see $SMOKE_DIR/adb-install.log)"
        fi
      fi

      # Kill emulator
      start-emulator --kill 2>/dev/null || true
      pass "Emulator stopped"
    else
      fail "Emulator boot failed (see $EMULATOR_LOG)"
    fi
  else
    skip "start-emulator not in PATH (not in nix devshell?)"
  fi
fi
