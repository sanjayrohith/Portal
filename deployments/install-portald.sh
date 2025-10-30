#!/usr/bin/env bash
# install-portald.sh — install portald as a resilient systemd background service.
set -euo pipefail

BIN_SRC="${1:-./portald}"
CONFIG_SRC="${2:-./deployments/portald.yaml}"
BIN_DST="/usr/local/bin/portald"
CONFIG_DIR="/etc/portald"
DATA_DIR="/var/lib/portald"

if [[ $EUID -ne 0 ]]; then
  echo "run as root: sudo $0 [binary] [config]" >&2
  exit 1
fi

if ! id portald >/dev/null 2>&1; then
  useradd --system --no-create-home --shell /usr/sbin/nologin portald
fi

install -m 0755 "$BIN_SRC" "$BIN_DST"
mkdir -p "$CONFIG_DIR" "$DATA_DIR"
if [[ -f "$CONFIG_SRC" ]]; then
  install -m 0600 "$CONFIG_SRC" "$CONFIG_DIR/portald.yaml"
fi
chown -R portald:portald "$DATA_DIR"
chown -R root:portald "$CONFIG_DIR"
chmod 0750 "$CONFIG_DIR"

install -m 0644 ./deployments/portald.service /etc/systemd/system/portald.service
systemctl daemon-reload
systemctl enable --now portald
systemctl status portald --no-pager || true

cat <<'EOF'
portald installed. Next steps:
  - Point wildcard DNS *.yourdomain.com at this host.
  - Open firewall: ufw allow 80,443/tcp && ufw allow 8443/tcp
  - Edit /etc/portald/portald.yaml then: systemctl reload portald  (SIGHUP, no restart)
  - Check health: curl localhost:9090/healthz && curl localhost:9090/readyz
EOF
