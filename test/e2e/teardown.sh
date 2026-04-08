#!/usr/bin/env bash
# teardown.sh — Stop backend services started by setup.sh.
#
# Called by the spec-kit parallel runner after the MCP E2E loop finishes.
# Safe to call even if services aren't running.
#
set -euo pipefail

PROJECT_DIR="${E2E_PROJECT_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
STATE_DIR="$PROJECT_DIR/test/e2e/.state"

log() { echo "[e2e-teardown] $(date +%H:%M:%S) $*"; }

if [ ! -d "$STATE_DIR" ]; then
  log "No state directory — nothing to tear down"
  exit 0
fi

# Stop services in reverse order
for service in daemon tailscaled headscale; do
  pidfile="$STATE_DIR/${service}.pid"
  if [ -f "$pidfile" ]; then
    pid=$(cat "$pidfile")
    if kill -0 "$pid" 2>/dev/null; then
      log "Stopping $service (PID $pid)"
      kill "$pid" 2>/dev/null || true
      # Wait up to 5s for graceful shutdown
      for _ in $(seq 1 5); do
        kill -0 "$pid" 2>/dev/null || break
        sleep 1
      done
      # Force kill if still alive
      kill -0 "$pid" 2>/dev/null && kill -9 "$pid" 2>/dev/null || true
    fi
    rm -f "$pidfile"
  fi
done

# Clean up state
rm -f "$STATE_DIR/setup.done" "$STATE_DIR/env"

# Clean up work directory
if [ -d "$STATE_DIR/work" ]; then
  rm -rf "$STATE_DIR/work"
fi

rmdir "$STATE_DIR" 2>/dev/null || true

log "All E2E backend services stopped"
