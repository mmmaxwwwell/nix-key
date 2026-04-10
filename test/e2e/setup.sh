#!/usr/bin/env bash
# setup.sh — Start backend services for nix-key E2E testing.
#
# Called by the spec-kit parallel runner before the MCP E2E loop starts.
# Starts: headscale, tailscaled (host), nix-key daemon.
# Does NOT start the emulator — the runner's PlatformManager handles that.
#
# Idempotent: safe to call if services are already running.
# Exports connection info to $STATE_DIR/env for the explore agent.
#
set -euo pipefail

PROJECT_DIR="${E2E_PROJECT_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"

# Re-exec under nix develop if tools aren't in PATH
if ! command -v headscale &>/dev/null; then
  exec nix develop "$PROJECT_DIR" --command bash "$0" "$@"
fi
STATE_DIR="$PROJECT_DIR/test/e2e/.state"

# --- Idempotency check ---
if [ -f "$STATE_DIR/setup.done" ]; then
  # Verify services are still alive
  all_alive=true
  for pidfile in headscale.pid tailscaled.pid daemon.pid; do
    if [ -f "$STATE_DIR/$pidfile" ]; then
      pid=$(cat "$STATE_DIR/$pidfile")
      if ! kill -0 "$pid" 2>/dev/null; then
        all_alive=false
        break
      fi
    else
      all_alive=false
      break
    fi
  done
  if [ "$all_alive" = "true" ]; then
    echo "E2E backend services already running"
    exit 0
  fi
  # Stale state — clean up and restart
  bash "$PROJECT_DIR/test/e2e/teardown.sh" 2>/dev/null || true
fi

mkdir -p "$STATE_DIR"

log() { echo "[e2e-setup] $(date +%H:%M:%S) $*"; }

wait_for() {
  local name="$1" cmd="$2" timeout="$3"
  local elapsed=0
  while [ $elapsed -lt "$timeout" ]; do
    if eval "$cmd" >/dev/null 2>&1; then
      log "$name ready (${elapsed}s)"
      return 0
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done
  log "FAIL: $name not ready after ${timeout}s" >&2
  return 1
}

# --- Configuration ---
HEADSCALE_PORT=18080
HEADSCALE_NAMESPACE="nixkey-e2e"
WORK_DIR="$STATE_DIR/work"
mkdir -p "$WORK_DIR"

# ── 1. Headscale ──
log "Starting headscale..."

HEADSCALE_DIR="$WORK_DIR/headscale"
mkdir -p "$HEADSCALE_DIR"

cat > "$HEADSCALE_DIR/config.yaml" <<EOF
server_url: http://127.0.0.1:${HEADSCALE_PORT}
listen_addr: 127.0.0.1:${HEADSCALE_PORT}
metrics_listen_addr: 127.0.0.1:0
private_key_path: ${HEADSCALE_DIR}/private.key
noise:
  private_key_path: ${HEADSCALE_DIR}/noise_private.key
unix_socket: ${HEADSCALE_DIR}/headscale.sock
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
echo $! > "$STATE_DIR/headscale.pid"

wait_for "headscale" "curl -sf http://127.0.0.1:${HEADSCALE_PORT}/health" 30

# Create user
headscale --config "$HEADSCALE_DIR/config.yaml" \
  users create "$HEADSCALE_NAMESPACE" 2>/dev/null || true

# Get user ID
user_id=$(headscale --config "$HEADSCALE_DIR/config.yaml" \
  users list -o json 2>/dev/null | \
  python3 -c "import sys,json; users=json.load(sys.stdin); print(next(u['id'] for u in users if u['name']=='$HEADSCALE_NAMESPACE'))" 2>/dev/null || echo "$HEADSCALE_NAMESPACE")

# Create pre-auth keys
HOST_AUTH_KEY=$(headscale --config "$HEADSCALE_DIR/config.yaml" \
  preauthkeys create --user "$user_id" --reusable --expiration 1h 2>/dev/null | tail -1)

PHONE_AUTH_KEY=$(headscale --config "$HEADSCALE_DIR/config.yaml" \
  preauthkeys create --user "$user_id" --reusable --expiration 1h 2>/dev/null | tail -1)

if [ -z "$HOST_AUTH_KEY" ] || [ -z "$PHONE_AUTH_KEY" ]; then
  log "FAIL: Could not create headscale pre-auth keys" >&2
  exit 1
fi

log "Headscale running (PID $(cat "$STATE_DIR/headscale.pid"))"

# ── 2. Host tailscaled ──
log "Starting tailscaled..."

TS_HOST_DIR="$WORK_DIR/ts-host"
mkdir -p "$TS_HOST_DIR"

tailscaled \
  --state="$TS_HOST_DIR/tailscaled.state" \
  --socket="$TS_HOST_DIR/tailscaled.sock" \
  --tun=userspace-networking \
  &>"$WORK_DIR/tailscaled.log" &
echo $! > "$STATE_DIR/tailscaled.pid"

sleep 2

tailscale --socket="$TS_HOST_DIR/tailscaled.sock" up \
  --login-server "http://127.0.0.1:${HEADSCALE_PORT}" \
  --auth-key "$HOST_AUTH_KEY" \
  --hostname "e2e-host" \
  --accept-routes=false \
  &>"$WORK_DIR/tailscale-up.log" 2>&1

wait_for "tailscale" "tailscale --socket=$TS_HOST_DIR/tailscaled.sock ip -4" 30

HOST_TAILSCALE_IP=$(tailscale --socket="$TS_HOST_DIR/tailscaled.sock" ip -4)
log "Host tailscale IP: $HOST_TAILSCALE_IP"

# ── 3. nix-key daemon ──
log "Starting nix-key daemon..."

DAEMON_DIR="$WORK_DIR/daemon"
mkdir -p "$DAEMON_DIR/certs" "$DAEMON_DIR/config"

export XDG_CONFIG_HOME="$DAEMON_DIR/xdg-config"
export XDG_STATE_HOME="$DAEMON_DIR/xdg-state"
export XDG_RUNTIME_DIR="$DAEMON_DIR/xdg-runtime"
mkdir -p "$XDG_CONFIG_HOME/nix-key" "$XDG_STATE_HOME/nix-key" "$XDG_RUNTIME_DIR/nix-key"

CONTROL_SOCKET="$XDG_RUNTIME_DIR/nix-key/control.sock"
SSH_AUTH_SOCK="$DAEMON_DIR/agent.sock"
AGE_KEY_FILE="$DAEMON_DIR/age-identity.txt"

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

echo '[]' > "$XDG_STATE_HOME/nix-key/devices.json"

go run "$PROJECT_DIR/cmd/nix-key" daemon &>"$WORK_DIR/daemon.log" &
echo $! > "$STATE_DIR/daemon.pid"

wait_for "daemon" "test -S $SSH_AUTH_SOCK" 10

log "nix-key daemon running (PID $(cat "$STATE_DIR/daemon.pid"))"

# ── Export connection info ──
cat > "$STATE_DIR/env" <<EOF
HEADSCALE_PORT=${HEADSCALE_PORT}
HEADSCALE_CONFIG=${HEADSCALE_DIR}/config.yaml
HOST_AUTH_KEY=${HOST_AUTH_KEY}
PHONE_AUTH_KEY=${PHONE_AUTH_KEY}
HOST_TAILSCALE_IP=${HOST_TAILSCALE_IP}
SSH_AUTH_SOCK=${SSH_AUTH_SOCK}
CONTROL_SOCKET=${CONTROL_SOCKET}
TAILSCALE_SOCKET=${TS_HOST_DIR}/tailscaled.sock
XDG_CONFIG_HOME=${XDG_CONFIG_HOME}
XDG_STATE_HOME=${XDG_STATE_HOME}
XDG_RUNTIME_DIR=${XDG_RUNTIME_DIR}
EOF

touch "$STATE_DIR/setup.done"
log "All E2E backend services ready"
log "Connection info written to $STATE_DIR/env"
