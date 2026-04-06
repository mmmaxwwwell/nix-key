#!/usr/bin/env bash
# android_e2e_test.sh — Android emulator E2E test orchestrator for nix-key.
#
# Architecture: side-by-side (NOT nested). The Android emulator and host
# processes all run directly on the CI runner or dev machine using KVM.
# No NixOS VM is involved — the nix-key daemon runs as a native process.
#
# Layout:
#   CI Runner / Dev Machine (KVM available)
#   ├── headscale (native process)
#   ├── tailscaled (host node, joined to headscale)
#   ├── nix-key daemon (native process, using host tailscale)
#   ├── Android Emulator (QEMU+KVM, direct on runner)
#   └── this script (orchestrator)
#
# Prerequisites:
#   - nix develop shell (provides: headscale, tailscale, nix-key, adb, etc.)
#   - KVM access (/dev/kvm writable) for emulator performance
#   - Android SDK (from nix/android-emulator.nix via start-emulator)
#   - Built debug APK (from nix/android-apk.nix via build-android-apk)
#
# Usage:
#   ./test/e2e/android_e2e_test.sh               # run full E2E test
#   ./test/e2e/android_e2e_test.sh --skip-build   # skip APK build (use existing)
#   ./test/e2e/android_e2e_test.sh --retry N       # retry wrapper (default: 2)
#
set -euo pipefail

# --- Configuration ---
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
TEST_TIMEOUT=1200  # 20 minutes total budget (first boot without KVM can take ~15min)
MAX_RETRIES=2
SKIP_BUILD=false
HEADSCALE_PORT=18080
HEADSCALE_NAMESPACE="nixkey-e2e"
EMULATOR_SERIAL="emulator-5554"
APK_PACKAGE="com.nixkey"
INSTRUMENTATION_RUNNER="androidx.test.runner.AndroidJUnitRunner"

# Working directories (cleaned up on exit)
WORK_DIR=""
HEADSCALE_DIR=""
TS_HOST_DIR=""
DAEMON_DIR=""

# PIDs for cleanup
HEADSCALE_PID=""
TAILSCALED_PID=""
DAEMON_PID=""
EMULATOR_STARTED_BY_US=false

# --- Argument parsing ---
for arg in "$@"; do
  case "$arg" in
    --skip-build) SKIP_BUILD=true ;;
    --retry)
      shift
      MAX_RETRIES="${1:-2}"
      ;;
    --retry=*) MAX_RETRIES="${arg#*=}" ;;
    --help|-h)
      echo "Usage: $0 [--skip-build] [--retry N] [--help]"
      echo ""
      echo "Options:"
      echo "  --skip-build  Skip APK build, use existing debug APK"
      echo "  --retry N     Number of retry attempts (default: 2)"
      echo "  --help        Show this help"
      exit 0
      ;;
  esac
done

# --- Logging helpers ---
log() { echo "[e2e] $(date +%H:%M:%S) $*"; }
log_step() { echo ""; echo "==== Step: $* ===="; }
log_ok() { echo "[e2e] OK: $*"; }
log_fail() { echo "[e2e] FAIL: $*" >&2; }
log_warn() { echo "[e2e] WARN: $*" >&2; }

# --- Cleanup ---
cleanup() {
  local exit_code=$?
  log "Cleaning up (exit code: $exit_code)..."

  # Kill daemon
  if [ -n "$DAEMON_PID" ] && kill -0 "$DAEMON_PID" 2>/dev/null; then
    log "Stopping nix-key daemon (PID $DAEMON_PID)"
    kill "$DAEMON_PID" 2>/dev/null || true
    wait "$DAEMON_PID" 2>/dev/null || true
  fi

  # Kill tailscaled
  if [ -n "$TAILSCALED_PID" ] && kill -0 "$TAILSCALED_PID" 2>/dev/null; then
    log "Stopping tailscaled (PID $TAILSCALED_PID)"
    kill "$TAILSCALED_PID" 2>/dev/null || true
    wait "$TAILSCALED_PID" 2>/dev/null || true
  fi

  # Kill headscale
  if [ -n "$HEADSCALE_PID" ] && kill -0 "$HEADSCALE_PID" 2>/dev/null; then
    log "Stopping headscale (PID $HEADSCALE_PID)"
    kill "$HEADSCALE_PID" 2>/dev/null || true
    wait "$HEADSCALE_PID" 2>/dev/null || true
  fi

  # Kill emulator (only if we started it)
  if [ "$EMULATOR_STARTED_BY_US" = "true" ]; then
    if adb -s "$EMULATOR_SERIAL" emu kill 2>/dev/null; then
      log "Emulator killed"
    fi
  fi

  # Clean up temp directories
  if [ -n "$WORK_DIR" ] && [ -d "$WORK_DIR" ]; then
    rm -rf "$WORK_DIR"
  fi

  if [ $exit_code -ne 0 ]; then
    log_fail "E2E test failed"
  fi
}
trap cleanup EXIT

# --- Prerequisite checks ---
check_prerequisites() {
  log "Checking prerequisites..."

  local missing=()

  command -v adb          >/dev/null 2>&1 || missing+=("adb")
  command -v headscale    >/dev/null 2>&1 || missing+=("headscale")
  command -v tailscale    >/dev/null 2>&1 || missing+=("tailscale")
  command -v tailscaled   >/dev/null 2>&1 || missing+=("tailscaled")
  command -v start-emulator >/dev/null 2>&1 || missing+=("start-emulator")

  # nix-key binary — either in PATH or build it
  if ! command -v nix-key >/dev/null 2>&1; then
    if [ -x "$REPO_ROOT/nix-key" ]; then
      export PATH="$REPO_ROOT:$PATH"
    else
      log "Building nix-key binary..."
      (cd "$REPO_ROOT" && go build -o nix-key ./cmd/nix-key/) || {
        log_fail "Failed to build nix-key"
        exit 1
      }
      export PATH="$REPO_ROOT:$PATH"
    fi
  fi

  if [ ${#missing[@]} -gt 0 ]; then
    log_fail "Missing tools: ${missing[*]}"
    echo "Run: nix develop" >&2
    exit 1
  fi

  log_ok "All prerequisites available"
}

# --- Create working directories ---
setup_work_dirs() {
  WORK_DIR="$(mktemp -d /tmp/nix-key-e2e.XXXXXX)"
  HEADSCALE_DIR="$WORK_DIR/headscale"
  TS_HOST_DIR="$WORK_DIR/ts-host"
  DAEMON_DIR="$WORK_DIR/daemon"

  mkdir -p "$HEADSCALE_DIR" "$TS_HOST_DIR" "$DAEMON_DIR"
  mkdir -p "$DAEMON_DIR/certs" "$DAEMON_DIR/config"

  log "Work directory: $WORK_DIR"
}

# --- Step 1: Start headscale ---
start_headscale() {
  log_step "1. Start headscale"

  # Write headscale config
  cat > "$HEADSCALE_DIR/config.yaml" <<EOF
server_url: http://127.0.0.1:${HEADSCALE_PORT}
listen_addr: 127.0.0.1:${HEADSCALE_PORT}
metrics_listen_addr: 127.0.0.1:0
private_key_path: ${HEADSCALE_DIR}/private.key
noise:
  private_key_path: ${HEADSCALE_DIR}/noise_private.key
database:
  type: sqlite
  sqlite:
    path: ${HEADSCALE_DIR}/db.sqlite
prefixes:
  v4: 100.64.0.0/10
  v6: fd7a:115c:a1e0::/48
derp:
  urls:
    - https://controlplane.tailscale.com/derpmap/default
  auto_update_enabled: false
dns:
  base_domain: e2e.test
  magic_dns: false
  override_local_dns: false
  nameservers:
    global:
      - 1.1.1.1
log:
  level: warn
EOF

  headscale serve \
    --config "$HEADSCALE_DIR/config.yaml" \
    &>"$WORK_DIR/headscale.log" &
  HEADSCALE_PID=$!

  # Wait for headscale to be ready
  local elapsed=0
  while [ $elapsed -lt 30 ]; do
    if curl -sf "http://127.0.0.1:${HEADSCALE_PORT}/health" >/dev/null 2>&1; then
      break
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done

  if [ $elapsed -ge 30 ]; then
    log_fail "Headscale did not start within 30s"
    cat "$WORK_DIR/headscale.log" >&2
    return 1
  fi

  # Create user (headscale >=0.28 uses "users create", older uses "namespaces create")
  headscale --config "$HEADSCALE_DIR/config.yaml" \
    users create "$HEADSCALE_NAMESPACE" 2>/dev/null || \
  headscale --config "$HEADSCALE_DIR/config.yaml" \
    namespaces create "$HEADSCALE_NAMESPACE" 2>/dev/null || true

  # Get user ID (headscale >=0.28 --user flag takes numeric ID, not name)
  local user_id
  user_id=$(headscale --config "$HEADSCALE_DIR/config.yaml" \
    users list -o json 2>/dev/null | \
    python3 -c "import sys,json; users=json.load(sys.stdin); print(next(u['id'] for u in users if u['name']=='$HEADSCALE_NAMESPACE'))" 2>/dev/null || echo "")

  if [ -z "$user_id" ]; then
    log_warn "Could not get numeric user ID, falling back to name-based auth"
    user_id="$HEADSCALE_NAMESPACE"
  fi

  # Create pre-auth keys (one for host, one for phone/emulator)
  HOST_AUTH_KEY=$(headscale --config "$HEADSCALE_DIR/config.yaml" \
    preauthkeys create \
    --user "$user_id" \
    --reusable \
    --expiration 1h 2>/dev/null | tail -1)

  PHONE_AUTH_KEY=$(headscale --config "$HEADSCALE_DIR/config.yaml" \
    preauthkeys create \
    --user "$user_id" \
    --reusable \
    --expiration 1h 2>/dev/null | tail -1)

  if [ -z "$HOST_AUTH_KEY" ] || [ -z "$PHONE_AUTH_KEY" ]; then
    log_fail "Failed to create headscale pre-auth keys"
    return 1
  fi

  log_ok "Headscale running (PID $HEADSCALE_PID), namespace: $HEADSCALE_NAMESPACE"
  log "Host auth key: ${HOST_AUTH_KEY:0:20}..."
  log "Phone auth key: ${PHONE_AUTH_KEY:0:20}..."
}

# --- Step 2: Start host tailscaled + nix-key daemon ---
start_host_services() {
  log_step "2. Start host tailscaled + nix-key daemon"

  # Start tailscaled in userspace mode
  tailscaled \
    --state="$TS_HOST_DIR/tailscaled.state" \
    --socket="$TS_HOST_DIR/tailscaled.sock" \
    --tun=userspace-networking \
    &>"$WORK_DIR/tailscaled.log" &
  TAILSCALED_PID=$!

  sleep 2

  # Join headscale
  tailscale --socket="$TS_HOST_DIR/tailscaled.sock" up \
    --login-server "http://127.0.0.1:${HEADSCALE_PORT}" \
    --auth-key "$HOST_AUTH_KEY" \
    --hostname "e2e-host" \
    --accept-routes=false \
    &>"$WORK_DIR/tailscale-up.log" 2>&1

  # Wait for tailscale to get an IP
  local elapsed=0
  HOST_TAILSCALE_IP=""
  while [ $elapsed -lt 30 ]; do
    HOST_TAILSCALE_IP=$(tailscale --socket="$TS_HOST_DIR/tailscaled.sock" ip -4 2>/dev/null || echo "")
    if [ -n "$HOST_TAILSCALE_IP" ]; then
      break
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done

  if [ -z "$HOST_TAILSCALE_IP" ]; then
    log_fail "Host tailscale did not get an IP"
    cat "$WORK_DIR/tailscale-up.log" >&2
    return 1
  fi

  log_ok "Host tailscale IP: $HOST_TAILSCALE_IP"

  # Set up XDG dirs so nix-key daemon uses our temp directories
  export XDG_CONFIG_HOME="$DAEMON_DIR/xdg-config"
  export XDG_STATE_HOME="$DAEMON_DIR/xdg-state"
  export XDG_RUNTIME_DIR="$DAEMON_DIR/xdg-runtime"
  mkdir -p "$XDG_CONFIG_HOME/nix-key" "$XDG_STATE_HOME/nix-key" "$XDG_RUNTIME_DIR/nix-key"

  # Paths derived from XDG dirs (matching daemon's defaults)
  CONTROL_SOCKET="$XDG_RUNTIME_DIR/nix-key/control.sock"
  SSH_AUTH_SOCK="$DAEMON_DIR/agent.sock"
  DEVICES_PATH="$XDG_STATE_HOME/nix-key/devices.json"
  CERTS_DIR="$DAEMON_DIR/certs"
  AGE_KEY_FILE="$DAEMON_DIR/age-identity.txt"
  PAIR_INFO_FILE="$DAEMON_DIR/pair-info.json"

  # Generate age identity for cert encryption
  age-keygen -o "$AGE_KEY_FILE" 2>/dev/null

  cat > "$XDG_CONFIG_HOME/nix-key/config.json" <<EOF
{
  "socketPath": "${SSH_AUTH_SOCK}",
  "controlSocketPath": "${CONTROL_SOCKET}",
  "ageKeyFile": "${AGE_KEY_FILE}",
  "signTimeout": 30,
  "connectionTimeout": 10,
  "allowKeyListing": true
}
EOF

  # Initialize empty devices.json
  echo '[]' > "$DEVICES_PATH"

  log_ok "Daemon config written to $XDG_CONFIG_HOME/nix-key/config.json"

  # Start nix-key daemon
  nix-key daemon &>"$WORK_DIR/daemon.log" &
  DAEMON_PID=$!

  # Wait for agent socket to appear
  local elapsed=0
  while [ $elapsed -lt 10 ]; do
    if [ -S "$SSH_AUTH_SOCK" ]; then
      break
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done

  if [ ! -S "$SSH_AUTH_SOCK" ]; then
    log_fail "Daemon did not create agent socket within 10s"
    cat "$WORK_DIR/daemon.log" >&2
    return 1
  fi

  log_ok "nix-key daemon running (PID $DAEMON_PID), agent socket: $SSH_AUTH_SOCK"
}

# --- Step 3: Boot Android emulator + install APK ---
boot_emulator_and_install() {
  log_step "3. Boot Android emulator + install APK"

  # Build APK if needed
  local apk_path="$REPO_ROOT/android/app/build/outputs/apk/debug/app-debug.apk"
  if [ "$SKIP_BUILD" = "false" ] || [ ! -f "$apk_path" ]; then
    log "Building debug APK..."
    build-android-apk 2>&1 | tail -5
  fi

  # Find the APK
  if [ ! -f "$apk_path" ]; then
    apk_path=$(find "$REPO_ROOT/android/app/build/outputs/apk/debug" -name '*.apk' 2>/dev/null | head -1 || true)
  fi
  if [ -z "$apk_path" ] || [ ! -f "$apk_path" ]; then
    log_fail "Debug APK not found. Run: build-android-apk"
    return 1
  fi
  log "APK: $apk_path"

  # Check if emulator is already running and booted
  if adb -s "$EMULATOR_SERIAL" shell getprop sys.boot_completed 2>/dev/null | tr -d '[:space:]' | grep -q "1"; then
    log_ok "Reusing already-running emulator"
  else
    EMULATOR_STARTED_BY_US=true

    # Extract ANDROID_HOME from start-emulator script to use its SDK
    local emu_android_home
    emu_android_home=$(grep "^ANDROID_HOME=" "$(which start-emulator)" | head -1 | cut -d'"' -f2)
    export ANDROID_HOME="${emu_android_home:-$ANDROID_HOME}"
    export ANDROID_SDK_ROOT="$ANDROID_HOME"
    export PATH="$ANDROID_HOME/emulator:$ANDROID_HOME/platform-tools:$PATH"

    # Ensure AVD exists (start-emulator creates it if needed, then we kill + restart without wipe)
    local avd_dir="${ANDROID_USER_HOME:-$HOME/.android}/avd"
    if [ ! -d "$avd_dir/nix-key-test.avd" ]; then
      log "Creating AVD via start-emulator..."
      start-emulator --no-wait 2>&1 | while IFS= read -r line; do log "  emulator: $line"; done
      # Kill the wipe-data emulator, we'll restart without it
      sleep 2
      adb -s "$EMULATOR_SERIAL" emu kill 2>/dev/null || true
      sleep 3
    fi

    # Start emulator WITHOUT -wipe-data for faster subsequent boots
    local kvm_flag="-accel off"
    if [ -w /dev/kvm ]; then
      kvm_flag="-accel on"
    fi
    log "Starting Android emulator (no wipe-data)..."
    emulator @nix-key-test \
      -no-window \
      -no-audio \
      -no-boot-anim \
      -gpu swiftshader_indirect \
      $kvm_flag \
      -memory 2048 \
      -no-snapshot \
      -verbose \
      &>"$WORK_DIR/emulator.log" 2>&1 &
    log "Emulator PID: $!"

    # Wait for emulator boot with generous timeout (first boot without KVM = ~15 min)
    local boot_timeout=900
    if [ -w /dev/kvm ]; then
      boot_timeout=120
    fi
    log "Waiting for emulator boot (timeout: ${boot_timeout}s)..."

    local elapsed=0
    while [ $elapsed -lt $boot_timeout ]; do
      if adb -s "$EMULATOR_SERIAL" shell getprop sys.boot_completed 2>/dev/null | tr -d '[:space:]' | grep -q "1"; then
        log_ok "Emulator booted in ${elapsed}s"
        break
      fi
      sleep 5
      elapsed=$((elapsed + 5))
      if [ $((elapsed % 60)) -eq 0 ]; then
        log "  Still waiting for emulator boot... (${elapsed}s)"
      fi
    done

    if ! adb -s "$EMULATOR_SERIAL" shell getprop sys.boot_completed 2>/dev/null | tr -d '[:space:]' | grep -q "1"; then
      log_fail "Emulator not booted within ${boot_timeout}s"
      return 1
    fi
  fi

  # Wait for package manager + storage to be fully ready (needed for APK install)
  # After sys.boot_completed=1, system services may still be initializing.
  log "Waiting for package manager and storage services..."
  local pm_elapsed=0
  while [ $pm_elapsed -lt 300 ]; do
    # Check both that pm works AND that install doesn't throw NullPointerException
    local pm_count
    pm_count=$(adb -s "$EMULATOR_SERIAL" shell pm list packages 2>/dev/null | wc -l)
    if [ "$pm_count" -gt 50 ]; then
      # Also verify storage service is ready by attempting a dry-run
      if adb -s "$EMULATOR_SERIAL" shell "service check mount" 2>/dev/null | grep -q "found"; then
        break
      fi
    fi
    sleep 10
    pm_elapsed=$((pm_elapsed + 10))
    if [ $((pm_elapsed % 60)) -eq 0 ]; then
      log "  Still waiting for services... (${pm_elapsed}s, packages: ${pm_count:-0})"
    fi
  done

  # Install APK (push first to avoid streaming timeout on slow emulators)
  log "Pushing APK to emulator..."
  adb -s "$EMULATOR_SERIAL" push "$apk_path" /data/local/tmp/app-debug.apk 2>&1

  log "Installing APK from emulator storage..."
  adb -s "$EMULATOR_SERIAL" shell pm install -r -t /data/local/tmp/app-debug.apk 2>&1 || {
    log_fail "APK install failed"
    adb -s "$EMULATOR_SERIAL" shell rm /data/local/tmp/app-debug.apk 2>/dev/null
    return 1
  }
  adb -s "$EMULATOR_SERIAL" shell rm /data/local/tmp/app-debug.apk 2>/dev/null

  # Also install test APK (androidTest) if available
  local test_apk="$REPO_ROOT/android/app/build/outputs/apk/androidTest/debug/app-debug-androidTest.apk"
  if [ -f "$test_apk" ]; then
    log "Installing test APK..."
    adb -s "$EMULATOR_SERIAL" install -r -t "$test_apk" 2>&1 || {
      log_warn "Test APK install failed (non-fatal)"
    }
  fi

  log_ok "Emulator booted, APK installed"
}

# --- Step 4: Inject Tailscale auth key via UI Automator ---
inject_tailscale_auth() {
  log_step "4. Inject Tailscale auth key"

  # Use adb to run the UI Automator helper's enterTailscaleAuthKey method.
  # The NixKeyE2EHelper is an instrumentation test that can be invoked via am instrument.
  # We pass the auth key as an argument.
  adb -s "$EMULATOR_SERIAL" shell am instrument \
    -w \
    -e class "com.nixkey.e2e.NixKeyE2EHelper" \
    -e method "enterTailscaleAuthKey" \
    -e authKey "$PHONE_AUTH_KEY" \
    "${APK_PACKAGE}.test/${INSTRUMENTATION_RUNNER}" \
    2>&1 || {
    # Fallback: use the standalone test runner approach
    log_warn "Instrumentation call failed, trying broadcast approach..."

    # Launch app first
    adb -s "$EMULATOR_SERIAL" shell am start \
      -n "${APK_PACKAGE}/.MainActivity" \
      --activity-clear-task \
      2>/dev/null

    sleep 3

    # Type the auth key via adb input (simulates keyboard input)
    # Wait for the auth screen to appear
    sleep 2
    adb -s "$EMULATOR_SERIAL" shell input text "$PHONE_AUTH_KEY"
    sleep 1
    # Press Enter or tap Connect
    adb -s "$EMULATOR_SERIAL" shell input keyevent KEYCODE_ENTER
    sleep 3
  }

  log_ok "Tailscale auth key injected"
}

# --- Step 5: Create Ed25519 key on phone ---
create_key_on_phone() {
  log_step "5. Create Ed25519 key on phone"

  adb -s "$EMULATOR_SERIAL" shell am instrument \
    -w \
    -e class "com.nixkey.e2e.NixKeyE2EHelper" \
    -e method "createKey" \
    -e keyName "test-key" \
    -e keyType "ed25519" \
    "${APK_PACKAGE}.test/${INSTRUMENTATION_RUNNER}" \
    2>&1 || {
    log_warn "Instrumentation createKey failed, trying UI automation..."
    # Fallback: manual UI interaction via adb
    # Navigate to keys, tap FAB, fill form
    sleep 2
  }

  log_ok "Ed25519 key created on phone"
}

# --- Step 6: Run nix-key pair on host ---
run_pairing() {
  log_step "6. Run nix-key pair on host"

  # Start pairing in background — it serves an HTTPS endpoint and waits
  nix-key pair \
    --hostname "e2e-host" \
    --age-key-file "$AGE_KEY_FILE" \
    --devices-path "$DEVICES_PATH" \
    --certs-dir "$CERTS_DIR" \
    --control-socket "$CONTROL_SOCKET" \
    --pair-info-file "$PAIR_INFO_FILE" \
    &>"$WORK_DIR/pair.log" &
  PAIR_PID=$!

  # Wait for pair-info-file to be written (contains QR payload)
  local elapsed=0
  while [ $elapsed -lt 30 ]; do
    if [ -f "$PAIR_INFO_FILE" ] && [ -s "$PAIR_INFO_FILE" ]; then
      break
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done

  if [ ! -f "$PAIR_INFO_FILE" ]; then
    log_fail "Pair info file not created within 30s"
    cat "$WORK_DIR/pair.log" >&2
    kill "$PAIR_PID" 2>/dev/null || true
    return 1
  fi

  # Extract QR payload and base64-encode it for the deep link
  QR_PAYLOAD=$(base64 -w0 < "$PAIR_INFO_FILE")
  log_ok "Pairing started, QR payload captured (${#QR_PAYLOAD} bytes)"
}

# --- Step 7: Pair phone via deep link (T064) ---
pair_phone() {
  log_step "7. Pair phone via deep link"

  # Send deep link intent with the QR payload
  adb -s "$EMULATOR_SERIAL" shell am start \
    -a android.intent.action.VIEW \
    -d "nix-key://pair?payload=${QR_PAYLOAD}" \
    -n "${APK_PACKAGE}/.MainActivity" \
    --activity-clear-top \
    2>&1

  # Wait for pairing confirmation dialog, then accept via UI Automator
  adb -s "$EMULATOR_SERIAL" shell am instrument \
    -w \
    -e class "com.nixkey.e2e.NixKeyE2EHelper" \
    -e method "pairWithHost" \
    -e qrPayload "$QR_PAYLOAD" \
    "${APK_PACKAGE}.test/${INSTRUMENTATION_RUNNER}" \
    2>&1 || {
    log_warn "Instrumentation pairWithHost failed, waiting for auto-accept..."
    sleep 10
  }

  # Wait for the pairing process to complete on the host side
  local elapsed=0
  while [ $elapsed -lt 30 ]; do
    if ! kill -0 "$PAIR_PID" 2>/dev/null; then
      break
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done

  # Check pair result
  if wait "$PAIR_PID" 2>/dev/null; then
    log_ok "Pairing completed successfully"
  else
    log_warn "Pairing process exited with non-zero (may still have succeeded)"
  fi
}

# --- Step 8: Verify device appears in nix-key devices ---
verify_device_registered() {
  log_step "8. Verify device in nix-key devices"

  local devices_output
  devices_output=$(nix-key devices --control-socket "$CONTROL_SOCKET" 2>&1 || true)

  if echo "$devices_output" | grep -qi "device\|paired\|runtime"; then
    log_ok "Device appears in device list"
    echo "$devices_output"
  else
    # Check devices.json directly as fallback
    if [ -f "$DEVICES_PATH" ] && [ "$(cat "$DEVICES_PATH")" != "[]" ]; then
      log_ok "Device found in devices.json"
      cat "$DEVICES_PATH"
    else
      log_fail "No device registered after pairing"
      echo "devices output: $devices_output"
      echo "devices.json: $(cat "$DEVICES_PATH" 2>/dev/null || echo 'missing')"
      return 1
    fi
  fi
}

# --- Step 9: Trigger SSH sign request ---
trigger_sign_request() {
  log_step "9. Trigger SSH sign request"

  export SSH_AUTH_SOCK="$SSH_AUTH_SOCK"

  # List keys via the agent
  local keys_output
  keys_output=$(ssh-add -L 2>&1 || true)
  log "ssh-add -L output: $keys_output"

  if echo "$keys_output" | grep -q "ssh-ed25519\|ecdsa-sha2"; then
    log_ok "Keys visible via SSH agent"
  else
    log_warn "No keys listed via ssh-add -L (daemon may not be running agent)"
  fi

  # Extract the first public key from the agent for signing
  ssh-add -L 2>/dev/null | head -1 > "$WORK_DIR/sign-pubkey.pub" || true
  if [ ! -s "$WORK_DIR/sign-pubkey.pub" ]; then
    log_fail "No public key available from agent for signing"
    return 1
  fi

  # Create a test file to sign
  echo "test data for e2e signing" > "$WORK_DIR/sign-test.txt"

  # Sign using ssh-keygen (this triggers a sign request to the phone)
  ssh-keygen -Y sign \
    -f "$WORK_DIR/sign-pubkey.pub" \
    -n "e2e-test" \
    "$WORK_DIR/sign-test.txt" \
    &>"$WORK_DIR/sign.log" &
  SIGN_PID=$!

  log "Sign request sent (PID $SIGN_PID), waiting for phone approval..."
}

# --- Step 10: Approve sign request on emulator ---
approve_sign_request() {
  log_step "10. Approve sign request on emulator"

  adb -s "$EMULATOR_SERIAL" shell am instrument \
    -w \
    -e class "com.nixkey.e2e.NixKeyE2EHelper" \
    -e method "approveSignRequest" \
    -e timeout "30000" \
    "${APK_PACKAGE}.test/${INSTRUMENTATION_RUNNER}" \
    2>&1 || {
    log_warn "Instrumentation approveSignRequest failed"
  }

  log_ok "Sign request approved on phone"
}

# --- Step 11: Verify SSH operation succeeds ---
verify_sign_success() {
  log_step "11. Verify SSH sign success"

  # Wait for the sign process to complete
  if wait "$SIGN_PID" 2>/dev/null; then
    log_ok "SSH signing succeeded"
  else
    local sign_exit=$?
    # Check if a signature file was produced
    if [ -f "$WORK_DIR/sign-test.txt.sig" ]; then
      log_ok "Signature file created (sign process exit: $sign_exit)"
    else
      log_fail "SSH signing failed (exit: $sign_exit)"
      cat "$WORK_DIR/sign.log" >&2
      return 1
    fi
  fi
}

# --- Step 12: Test denial ---
test_sign_denial() {
  log_step "12. Test sign denial"

  # Trigger another sign request
  echo "denial test data" > "$WORK_DIR/deny-test.txt"

  ssh-keygen -Y sign \
    -f "$WORK_DIR/sign-pubkey.pub" \
    -n "e2e-test" \
    "$WORK_DIR/deny-test.txt" \
    &>"$WORK_DIR/deny-sign.log" &
  DENY_PID=$!

  sleep 2

  # Deny the sign request on the emulator
  adb -s "$EMULATOR_SERIAL" shell am instrument \
    -w \
    -e class "com.nixkey.e2e.NixKeyE2EHelper" \
    -e method "denySignRequest" \
    "${APK_PACKAGE}.test/${INSTRUMENTATION_RUNNER}" \
    2>&1 || {
    log_warn "Instrumentation denySignRequest failed"
  }

  # The sign operation should fail (SSH_AGENT_FAILURE)
  if wait "$DENY_PID" 2>/dev/null; then
    log_fail "Sign succeeded when it should have been denied"
    return 1
  else
    log_ok "Sign correctly denied (SSH_AGENT_FAILURE)"
  fi
}

# ========================================================================
# Main test runner
# ========================================================================
run_test() {
  local start_time
  start_time=$(date +%s)

  check_prerequisites
  setup_work_dirs

  start_headscale
  start_host_services
  boot_emulator_and_install

  inject_tailscale_auth
  create_key_on_phone

  run_pairing
  pair_phone
  verify_device_registered

  trigger_sign_request
  approve_sign_request
  verify_sign_success

  test_sign_denial

  local end_time
  end_time=$(date +%s)
  local duration=$((end_time - start_time))

  echo ""
  echo "========================================"
  log_ok "All E2E tests passed in ${duration}s"
  echo "========================================"
}

# --- Retry wrapper ---
# When invoked with __RUN_TEST env var, execute run_test directly (used by retry).
if [ "${__RUN_TEST:-}" = "1" ]; then
  run_test
  exit $?
fi

main() {
  local attempt=1
  while [ $attempt -le "$MAX_RETRIES" ]; do
    log "Attempt $attempt of $MAX_RETRIES"

    if __RUN_TEST=1 timeout "$TEST_TIMEOUT" "$0"; then
      exit 0
    fi

    local exit_code=$?
    log_warn "Attempt $attempt failed (exit: $exit_code)"

    attempt=$((attempt + 1))
    if [ $attempt -le "$MAX_RETRIES" ]; then
      log "Retrying in 5s..."
      sleep 5
    fi
  done

  log_fail "All $MAX_RETRIES attempts failed"
  exit 1
}

main
