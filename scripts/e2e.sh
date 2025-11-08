#!/usr/bin/env bash
# scripts/e2e.sh — orchestration harness for the Portal client+server pair.
#
# Spins up a real portald daemon (temp config, loopback listeners), smoke
# checks the portal client binary, then runs every in-process end-to-end
# suite (all TestEndToEnd* cases, which pair an edge session with a bridged
# client session against mock local upstreams) as the validation pass
# against the tunnel path.
#
# NOTE: portald currently boots its control plane without binding public
# listeners, so daemon readiness here means a live process that logged
# startup (not an HTTP probe). Listener serving lands with the edge
# integration; the script already polls $ADMIN_ADDR/healthz first and falls
# back to process liveness so it keeps working once probes are served.
#
# Usage:
#   ./scripts/e2e.sh [-- <extra go test args>]
#   ./scripts/e2e.sh -- -count=1 -v ./pkg/proxy/
set -euo pipefail

ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="$ROOT/bin"
WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/portal-e2e.XXXXXX")"
DAEMON_LOG="$WORK_DIR/portald.log"
DAEMON_PID=""

ADMIN_ADDR="127.0.0.1:19090"

cleanup() {
  if [[ -n "$DAEMON_PID" ]] && kill -0 "$DAEMON_PID" 2>/dev/null; then
    kill "$DAEMON_PID" 2>/dev/null || true
    wait "$DAEMON_PID" 2>/dev/null || true
  fi
  rm -rf "$WORK_DIR"
}
trap cleanup EXIT

log() { printf '[e2e] %s\n' "$*"; }
die() { printf '[e2e] ERROR: %s\n' "$*" >&2; exit 1; }

# 1. Build both binaries (daemon + client).
log "building portal and portald..."
mkdir -p "$BIN_DIR"
go build -o "$BIN_DIR/portal" ./cmd/portal
go build -o "$BIN_DIR/portald" ./cmd/portald

# 2. Ephemeral daemon config on loopback-only addresses.
cat > "$WORK_DIR/portald.yaml" <<EOF
domain: "e2e.portal.test"
control_addr: "127.0.0.1:18443"
http_addr: "127.0.0.1:18080"
https_addr: "127.0.0.1:18444"
admin_addr: "$ADMIN_ADDR"
storage_type: "sqlite"
storage_dsn: "$WORK_DIR/e2e.db"
rate_limit_requests_per_sec: 100
max_concurrent_tunnels: 5
log_level: "info"
EOF

# 3. Boot the daemon.
log "starting portald..."
"$BIN_DIR/portald" --config "$WORK_DIR/portald.yaml" >"$DAEMON_LOG" 2>&1 &
DAEMON_PID=$!

# 4. Readiness: prefer the admin /healthz probe, fall back to process
#    liveness while listener serving is still under construction.
ready=0
for _ in $(seq 1 50); do
  if curl -fsS --max-time 1 "http://$ADMIN_ADDR/healthz" >/dev/null 2>&1; then
    log "daemon healthy at http://$ADMIN_ADDR/healthz"
    ready=1
    break
  fi
  if ! kill -0 "$DAEMON_PID" 2>/dev/null; then
    die "portald exited during startup; log:"$'\n'"$(cat "$DAEMON_LOG")"
  fi
  if grep -q "starting portal daemon" "$DAEMON_LOG" 2>/dev/null; then
    log "daemon live (process up, startup logged; no HTTP probe served yet)"
    ready=1
    break
  fi
  sleep 0.2
done
[[ "$ready" -eq 1 ]] || die "portald did not become ready; log:"$'\n'"$(cat "$DAEMON_LOG")"

# 5. Client smoke check: the CLI parses and runs (server pairing is covered
#    by the in-process suites below).
log "client smoke check..."
"$BIN_DIR/portal" --help >/dev/null
"$BIN_DIR/portal" http --help >/dev/null

# 6. Validation suites: every end-to-end client+server pair test.
log "running end-to-end suites (TestEndToEnd*)..."
# shellcheck disable=SC2086
go test -run 'TestEndToEnd' ./... "$@"

log "e2e harness green (daemon pid $DAEMON_PID reaped on exit)"
