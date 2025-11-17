# Self-Hosting Portal

Run your own Portal control plane and edge proxy on a single VPS. This guide
covers server sizing, wildcard DNS, firewall rules, TLS issuance, and client
onboarding. All paths, ports, and commands below match this repository.

## 1. Provision the VPS

Minimum viable server (dozens of concurrent tunnels):

| Resource | Minimum | Recommended |
| -------- | ------- | ----------- |
| vCPU     | 1       | 2           |
| Memory   | 1 GB    | 2 GB        |
| Disk     | 10 GB   | 25 GB (SQLite growth + logs) |
| OS       | Ubuntu 22.04+ / Debian 12+ | Same, with unattended upgrades |

```bash
apt update && apt upgrade -y
apt install -y ufw curl sqlite3
```

## 2. Wildcard DNS

Every tunnel is a subdomain of your base domain (e.g. `myapp.tunnel.example.com`),
so you need **two** A records pointing at the VPS public IP (say `203.0.113.10`):

| Host | Type | Value |
| ---- | ---- | ----- |
| `tunnel.example.com` | A | `203.0.113.10` |
| `*.tunnel.example.com` | A | `203.0.113.10` |

Verify propagation from your laptop before continuing:

```bash
dig +short tunnel.example.com
dig +short anything.tunnel.example.com   # must return the same IP
```

Local end-to-end testing without public DNS: use the compose environment's
mock resolver (`deployments/dnsmasq.conf`), e.g.
`dig @127.0.0.1 -p 5353 webhook.portal.test`.

## 3. Firewall

Allow SSH, public HTTP/HTTPS, and the control plane. Keep the admin/metrics
port off the public internet (it binds `127.0.0.1:9090` by default):

```bash
ufw default deny incoming
ufw default allow outgoing
ufw allow 22/tcp
ufw allow 80/tcp
ufw allow 443/tcp
ufw allow 8443/tcp   # portal client control connections
ufw enable && ufw status
```

Do **not** open 9090 publicly; scrape `/metrics` and `/healthz` via SSH
tunnel or a node-local agent.

## 4. Install portald

Build (or download a release binary, see `.goreleaser.yaml` artifacts) and run
the installer, which creates the `portald` system user, installs the systemd
unit, and enables the service:

```bash
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o portald ./cmd/portald
sudo ./deployments/install-portald.sh ./portald ./deployments/portald.yaml
```

What the installer does (`deployments/install-portald.sh`):

- installs `/usr/local/bin/portald` (0755)
- installs `/etc/portald/portald.yaml` (0600, owned `root:portald`)
- installs `deployments/portald.service` → `/etc/systemd/system/portald.service`
- `systemctl enable --now portald`

## 5. Configure

Edit `/etc/portald/portald.yaml` (see `deployments/portald.yaml` for the full
schema). The fields you must set:

```yaml
domain: "tunnel.example.com"
control_addr: ":8443"
http_addr: ":80"
https_addr: ":443"
admin_addr: "127.0.0.1:9090"   # keep loopback-only
storage_type: "sqlite"
storage_dsn: "/var/lib/portald/portal.db"
```

Apply without dropping tunnels (hot-reload preserves connections):

```bash
sudo systemctl reload portald   # sends SIGHUP: re-reads tokens + rate limits
```

## 6. TLS

Two options, matching the `cert_file`/`key_file`/`acme_email` settings:

**A. Static certificates** (you terminate TLS elsewhere or bring your own):

```yaml
cert_file: "/etc/portald/tls/fullchain.pem"
key_file: "/etc/portald/tls/privkey.pem"
```

**B. Automatic wildcard via ACME DNS-01** (recommended for `*.tunnel.example.com`):

```yaml
acme_email: "ops@example.com"
acme_dir: "/var/lib/portald/certs"
```

DNS-01 is required (not HTTP-01) because a wildcard cannot be validated over
HTTP. Create the API token at your DNS provider and follow your provider's
lego change process; portald renews in the background before expiry. For
offline/staging setups, generate a self-signed wildcard with the built-in
utility (`pkg/transport` self-signed generator) and distribute the CA to
test clients (`portal http --insecure` is development-only).

## 7. Issue client tokens

```bash
# On the server:
sudo portald tokens generate --owner alice --config /etc/portald/portald.yaml
# Persisted to the config file; applied live via:
sudo systemctl reload portald

# Audit later:
sudo portald tokens list --config /etc/portald/portald.yaml
sudo portald tunnels list --admin-addr 127.0.0.1:9090
```

Revoke with `portald tokens revoke <id> --config ...` followed by a reload.
Treat tokens as secrets: they are hashed in memory but stored plaintext in
the config file (0600).

## 8. Connect a client

```bash
portal http 3000 --subdomain myapp \
  --server tunnel.example.com:8443 \
  --token pt_<secret>
# Public URL: https://myapp.tunnel.example.com
```

The local inspector is always loopback-only: http://127.0.0.1:4040
(strict binding is enforced and audited; see the [Security Hardening Guide](security.md)).

## 9. Verify the deployment

```bash
systemctl status portald --no-pager
curl -sf http://127.0.0.1:9090/healthz && echo LIVENESS_OK
curl -sf http://127.0.0.1:9090/readyz  && echo READINESS_OK
curl -s -H 'Host: myapp.tunnel.example.com' http://127.0.0.1:80/ | head -c 200
```

## 10. Troubleshooting

| Symptom | Likely cause | Fix |
| ------- | ------------ | --- |
| Subdomain resolves nowhere | Wildcard A record missing | Re-check `dig *.tunnel.example.com` |
| `connection refused` on 8443 | ufw blocking / daemon down | `ufw status`; `journalctl -u portald` |
| `invalid token` on connect | Wrong token / not reloaded | `tokens list`, then `systemctl reload portald` |
| Browser TLS error | ACME DNS-01 incomplete | Check provider token, `acme_dir` writability |
| 404 branded page on subdomain | No active tunnel for that name | Client must be connected and holding the name |
| `/readyz` returns 503 | Listener or DB check failing | `journalctl -u portald`; check disk space |

Containerized alternative: `docker compose up --build -d` (see
`docker-compose.yml`) reproduces daemon + mock DNS + sample upstream locally.
