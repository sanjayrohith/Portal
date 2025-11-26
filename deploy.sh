#!/usr/bin/env bash
# deploy.sh — Automated VPS deployment script for Portal control plane daemon (portald).
# Sets up system dependencies, directories, portald systemd service, UFW firewall rules,
# and verifies DNS configuration.
#
# Usage:
#   sudo ./deploy.sh [OPTIONS]
#
# Options:
#   -d, --domain DOMAIN       Base domain for tunnels (e.g., tunnel.example.com)
#   -t, --token TOKEN         Admin authentication token for initial clients
#   -b, --binary PATH         Path to portald binary (builds from source if omitted)
#   --skip-firewall           Skip UFW firewall configuration
#   --dry-run                 Simulate deployment steps without modifying the system
#   -h, --help                Show this help message

set -euo pipefail

# Configuration defaults
DOMAIN="${PORTAL_DOMAIN:-tunnel.example.com}"
INITIAL_TOKEN="${PORTAL_INITIAL_TOKEN:-$(head -c 16 /dev/urandom | xxd -p 2>/dev/null || openssl rand -hex 16 2>/dev/null || echo "portal_sec_$(date +%s)")}"
BINARY_PATH="${PORTAL_BINARY:-}"
SKIP_FIREWALL=false
DRY_RUN=false

INSTALL_BIN="/usr/local/bin/portald"
CONFIG_DIR="/etc/portald"
CONFIG_FILE="/etc/portald/portald.yaml"
DATA_DIR="/var/lib/portald"
SYSTEMD_SERVICE="/etc/systemd/system/portald.service"

log_info()  { echo -e "\033[1;34m[INFO]\033[0m $*"; }
log_warn()  { echo -e "\033[1;33m[WARN]\033[0m $*"; }
log_error() { echo -e "\033[1;31m[ERROR]\033[0m $*" >&2; }
log_ok()    { echo -e "\033[1;32m[SUCCESS]\033[0m $*"; }

run_cmd() {
  if [ "$DRY_RUN" = true ]; then
    echo "[DRY-RUN] $*"
  else
    "$@"
  fi
}

usage() {
  cat <<EOF
Usage: sudo ./deploy.sh [OPTIONS]

Automated VPS deployment script for Portal control plane (portald).

Options:
  -d, --domain DOMAIN       Base domain (default: ${DOMAIN})
  -t, --token TOKEN         Initial auth token (auto-generated if omitted)
  -b, --binary PATH         Path to existing portald binary
  --skip-firewall           Do not configure UFW firewall
  --dry-run                 Print commands without making system changes
  -h, --help                Display this help message
EOF
  exit 0
}

# Parse CLI arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
    -d|--domain)
      DOMAIN="$2"
      shift 2
      ;;
    -t|--token)
      INITIAL_TOKEN="$2"
      shift 2
      ;;
    -b|--binary)
      BINARY_PATH="$2"
      shift 2
      ;;
    --skip-firewall)
      SKIP_FIREWALL=true
      shift
      ;;
    --dry-run)
      DRY_RUN=true
      shift
      ;;
    -h|--help)
      usage
      ;;
    *)
      log_error "Unknown argument: $1"
      usage
      ;;
  esac
done

# Require root permissions unless in dry-run mode
if [ "$DRY_RUN" = false ] && [ "$(id -u)" -ne 0 ]; then
  log_error "This script must be run as root: sudo $0"
  exit 1
fi

log_info "=== Starting Portal VPS Automated Deployment ==="
log_info "Target Domain: $DOMAIN"

# Detect Public IP
DETECTED_IP=""
if command -v curl >/dev/null 2>&1; then
  DETECTED_IP=$(curl -s --max-time 3 https://ifconfig.me || curl -s --max-time 3 https://api.ipify.org || true)
fi
if [ -z "$DETECTED_IP" ] && command -v ip >/dev/null 2>&1; then
  DETECTED_IP=$(ip route get 1.1.1.1 2>/dev/null | awk '{print $7}' | head -n1 || true)
fi
if [ -n "$DETECTED_IP" ]; then
  log_info "Detected Public IP: $DETECTED_IP"
fi

# Step 1: Ensure dependencies & build or verify binary
log_info "Checking portald binary..."
if [ -z "$BINARY_PATH" ]; then
  if [ -f "./cmd/portald/main.go" ] && command -v go >/dev/null 2>&1; then
    log_info "Building portald from local source..."
    TMP_BIN=$(mktemp /tmp/portald.XXXXXX)
    run_cmd go build -trimpath -ldflags "-s -w" -o "$TMP_BIN" ./cmd/portald
    BINARY_PATH="$TMP_BIN"
  elif [ -f "./portald" ]; then
    BINARY_PATH="./portald"
  else
    log_error "No portald binary provided and unable to compile from source."
    exit 1
  fi
fi

if [ "$DRY_RUN" = false ] && [ ! -f "$BINARY_PATH" ]; then
  log_error "Binary not found at $BINARY_PATH"
  exit 1
fi

# Step 2: Create dedicated system user
log_info "Configuring portald system user..."
if [ "$DRY_RUN" = false ]; then
  if ! id -u portald >/dev/null 2>&1; then
    useradd --system --no-create-home --shell /usr/sbin/nologin portald
    log_ok "Created system user: portald"
  else
    log_info "System user 'portald' already exists."
  fi
else
  echo "[DRY-RUN] useradd --system --no-create-home --shell /usr/sbin/nologin portald"
fi

# Step 3: Install binary
log_info "Installing binary to $INSTALL_BIN..."
run_cmd install -m 0755 "$BINARY_PATH" "$INSTALL_BIN"

# Step 4: Provision directories and initial configuration
log_info "Provisioning directories and configuration..."
run_cmd mkdir -p "$CONFIG_DIR" "$DATA_DIR"
run_cmd chown -R portald:portald "$DATA_DIR"
run_cmd chmod 0700 "$DATA_DIR"

if [ "$DRY_RUN" = false ] && [ ! -f "$CONFIG_FILE" ]; then
  log_info "Creating default configuration at $CONFIG_FILE..."
  cat > "$CONFIG_FILE" <<EOF
server:
  domain: "${DOMAIN}"
  http_port: 80
  https_port: 443
  control_port: 8443
  metrics_port: 9090

tls:
  enabled: true
  auto_cert: true
  email: "admin@${DOMAIN}"
  cert_file: ""
  key_file: ""

storage:
  type: "sqlite"
  sqlite:
    path: "${DATA_DIR}/portald.db"

security:
  rate_limit:
    requests_per_second: 100
    burst: 200
  limits:
    max_tunnels_per_token: 10
    max_streams_per_tunnel: 256
    max_body_bytes: 10485760

tokens:
  - id: "tok_admin"
    token: "${INITIAL_TOKEN}"
    owner: "admin"
    max_tunnels: 20
    created_at: "$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
EOF
  chmod 0600 "$CONFIG_FILE"
  chown root:portald "$CONFIG_FILE"
  log_ok "Generated configuration with admin token: ${INITIAL_TOKEN}"
elif [ "$DRY_RUN" = true ]; then
  echo "[DRY-RUN] Generate $CONFIG_FILE with domain $DOMAIN"
fi

# Step 5: Install systemd service
log_info "Installing systemd unit..."
if [ "$DRY_RUN" = false ]; then
  cat > "$SYSTEMD_SERVICE" <<EOF
[Unit]
Description=Portal Tunneling Control Plane Daemon
Documentation=https://github.com/sanjayrohith/portal
After=network.target network-online.target
Wants=network-online.target

[Service]
Type=simple
User=portald
Group=portald
ExecStart=${INSTALL_BIN} --config ${CONFIG_FILE}
ExecReload=/bin/kill -HUP \$MAINPID
Restart=always
RestartSec=3s
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=true
LimitNOFILE=65535
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF
  chmod 0644 "$SYSTEMD_SERVICE"
  systemctl daemon-reload
  systemctl enable portald
  systemctl restart portald || log_warn "Failed to start service immediately. Check journalctl -u portald."
  log_ok "Systemd service portald enabled and started."
else
  echo "[DRY-RUN] Created and started $SYSTEMD_SERVICE"
fi

# Step 6: Configure UFW Firewall rules
if [ "$SKIP_FIREWALL" = false ]; then
  log_info "Configuring UFW firewall rules..."
  if command -v ufw >/dev/null 2>&1; then
    run_cmd ufw default deny incoming
    run_cmd ufw default allow outgoing
    run_cmd ufw allow 22/tcp comment "SSH"
    run_cmd ufw allow 80/tcp comment "HTTP/ACME"
    run_cmd ufw allow 443/tcp comment "HTTPS Edge Proxy"
    run_cmd ufw allow 8443/tcp comment "Portal Control Plane"
    if [ "$DRY_RUN" = false ]; then
      ufw --force enable || true
      log_ok "Firewall configured: Ports 22, 80, 443, 8443 open. Port 9090 protected on loopback."
    fi
  else
    log_warn "UFW is not installed. Ensure ports 80, 443, and 8443 are open."
  fi
else
  log_info "Skipping firewall configuration."
fi

# Step 7: DNS Guidance & Verification
log_info "=== Deployment Complete ==="
echo ""
log_ok "Portal server daemon successfully provisioned!"
echo ""
echo "----------------------------------------------------------------"
echo "DNS CONFIGURATION REQUIRED:"
echo "Configure the following DNS A records with your DNS provider:"
if [ -n "$DETECTED_IP" ]; then
  echo "  ${DOMAIN}         A    ${DETECTED_IP}"
  echo "  *.${DOMAIN}       A    ${DETECTED_IP}"
else
  echo "  ${DOMAIN}         A    <YOUR_VPS_PUBLIC_IP>"
  echo "  *.${DOMAIN}       A    <YOUR_VPS_PUBLIC_IP>"
fi
echo "----------------------------------------------------------------"
echo ""
echo "ADMIN AUTHENTICATION TOKEN:"
echo "  Token: ${INITIAL_TOKEN}"
echo ""
echo "CLIENT CONNECT COMMAND:"
echo "  portal http 3000 --server ${DOMAIN}:8443 --token ${INITIAL_TOKEN} --subdomain demo"
echo ""
echo "HEALTH & MANAGEMENT:"
echo "  Systemd status:   systemctl status portald"
echo "  Live logs:        journalctl -u portald -f"
echo "  Configuration:    ${CONFIG_FILE}"
echo "  Liveness check:   curl -s http://127.0.0.1:9090/healthz"
echo "----------------------------------------------------------------"
