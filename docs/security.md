# Production Security Hardening Guide & Checklist

Exposing localhost services to the public internet creates a potential ingress
vector into private developer environments. This document provides a production
hardening guide, threat mitigation strategy, and operational checklist for
deploying the **Portal** server daemon (`portald`) and client agent (`portal`).

---

## 1. Threat Model & Security Posture

### Ingress Architecture

```
Internet Request
       │
       ▼
[ Cloud WAF / CDN ]  ◄── L3/L4/L7 DDoS Mitigation & Origin Cloaking
       │
       ▼ (Ports 80/443)
[ portald Edge Proxy ] ◄── Rate Limiting, CIDR Allowlisting, Basic Auth
       │
       ▼ (Port 8443 TLS 1.3)
[ Control Plane ]   ◄── Constant-time Token Auth, Quota Enforcement
       │
       ▼ (Multiplexed Stream)
[ portal Agent ]    ◄── Strict 127.0.0.1 Inspector, Loopback Upstream
```

### Key Risk Areas & Defenses

| Risk Area | Threat Vector | Portal Defense |
| :--- | :--- | :--- |
| **Control Plane** | Unauthorized tunnel hijacking | Mandatory per-token authentication with constant-time cryptographic verification. |
| **Edge Proxy** | Denial of Service (DoS) / Resource exhaustion | Per-token token bucket rate limiting, max payload caps, and stream limits. |
| **Public Endpoints** | Exposed staging or internal endpoints | Built-in HTTP Basic Authentication and CIDR IP allowlisting middleware. |
| **Inspector UI** | Leakage of webhook payloads and API secrets | Strict binding to `127.0.0.1` only; in-memory circular ring buffer (zero disk writes). |
| **Transport** | Eavesdropping and replay attacks | Enforced TLS 1.3 with ALPN `portal/1` negotiation and monotonic keepalive nonces. |

---

## 2. Edge Protection & Reverse-Proxy Fronting

### 2.1 Fronting with Cloudflare or AWS CloudFront

Never expose the raw VPS IP address if your tunneling daemon will handle
untrusted public traffic. Front `portald` with a cloud CDN / WAF:

1. **DNS Proxied Mode**: Proxy wildcard records (`*.tunnel.example.com`) through
   the CDN to absorb L3/L4 volumetric attacks.
2. **Origin Shielding**: Configure the VPS firewall to only accept connections
   on ports 80 and 443 from known Cloudflare/CDN IP ranges:

```bash
# Example: Allow only Cloudflare IP ranges on public web ports
for ip in $(curl -s https://www.cloudflare.com/ips-v4); do
    ufw allow proto tcp from "$ip" to any port 80,443 comment "Cloudflare"
done
```

> **Note**: Port 8443 (control plane) carries raw TLS 1.3 binary framing and
> must either be accessed directly by developer clients or routed via a TCP-level
> reverse proxy (e.g. Cloudflare Spectrum or HAProxy with TCP pass-through).

### 2.2 Header Sanitization & Injection Prevention

`portald` automatically sanitizes forwarded HTTP request headers:
- Untrusted hop-by-hop headers (`Connection`, `Keep-Alive`, `Proxy-Authenticate`,
  `Trailer`, `Upgrade`) are stripped.
- `X-Forwarded-For` is appended with the real remote peer IP.
- `X-Forwarded-Proto` and `X-Forwarded-Host` are normalized according to the
  incoming TLS termination state.

---

## 3. Authentication & Access Control

### 3.1 Token Security

- **Constant-Time Verification**: All token matching in `pkg/auth` uses
  `crypto/subtle.ConstantTimeCompare` to eliminate timing side-channel attacks.
- **Dynamic Hot-Reloading**: To revoke a compromised token or grant temporary
  access, edit `/etc/portald/portald.yaml` and trigger a hot reload without
  dropping existing connections:

```bash
systemctl reload portald   # Sends SIGHUP; reloads tokens & limits
```

### 3.2 Protecting Public Tunnel Endpoints

For sensitive internal APIs or webhooks under development, enforce endpoint
access controls:

#### HTTP Basic Authentication
Require credentials on the public tunnel URL:

```bash
portal http 3000 --subdomain staging --basic-auth "admin:secure_password"
```

#### CIDR IP Allowlisting
Restrict incoming requests to your office or VPN network:

```bash
portal http 3000 --subdomain staging --allow-ip "203.0.113.0/24,198.51.100.5"
```

---

## 4. Rate Limiting & Resource Bounds

Prevent misbehaving clients or external scrapers from overwhelming the control
plane:

```yaml
# /etc/portald/portald.yaml
security:
  rate_limit:
    requests_per_second: 100   # Token bucket refill rate
    burst: 200                 # Maximum burst capacity
  limits:
    max_tunnels_per_token: 10   # Prevent single-user tunnel exhaustion
    max_streams_per_tunnel: 256 # Prevent stream state explosion
    max_body_bytes: 10485760    # 10 MB maximum request payload cap
    max_header_bytes: 65536     # 64 KB header size boundary
```

- **Unresponsive Stream Pruning**: Streams that fail to read or write within
  idle timeout thresholds (default: 60s) are aborted with `FlagRst`.
- **Bounded Accept Queues**: The multiplexer session maintains a bounded accept
  queue of 256 pending streams; requests exceeding this limit receive an
  immediate `RST` to prevent unbounded memory growth.

---

## 5. Web Inspector Security (Workstation Side)

The built-in web inspector at `127.0.0.1:4040` displays full request/response
headers and payloads (which may include OAuth tokens, API keys, and PII):

1. **Loopback-Only Enforcement**: The inspector server strictly validates that
   its bind address is `127.0.0.1` or `localhost`. It categorically rejects
   binding to `0.0.0.0` or external network adapters.
2. **In-Memory Volatility**: Captured requests are retained in an in-memory
   circular ring buffer (default capacity: 200 requests). No payloads are ever
   written to local disk, temp files, or database caches.
3. **Process Isolation**: Closing `portal` immediately purges all buffered
   telemetry from workstation RAM.

---

## 6. Linux Host & Process Hardening

When deploying `portald` to a Linux VPS:

### 6.1 Run as Unprivileged System User

Never execute `portald` as `root`. The deployment script creates a system user
`portald` with no login shell:

```ini
[Service]
User=portald
Group=portald
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
```

`CAP_NET_BIND_SERVICE` allows binding ports 80 and 443 without root privileges.

### 6.2 Firewall Policy

Ensure administrative and telemetry endpoints are inaccessible externally:

```bash
# UFW Rules
ufw default deny incoming
ufw default allow outgoing
ufw allow 22/tcp    # SSH (prefer key-only authentication)
ufw allow 80/tcp    # HTTP (ACME challenge & edge redirect)
ufw allow 443/tcp   # HTTPS (Public edge proxy)
ufw allow 8443/tcp  # Portal control plane TLS
ufw enable
```

> **Warning**: Keep port `9090` (`/metrics` and `/healthz`) bound to
> `127.0.0.1:9090`. Do not expose port 9090 to the internet. Scrape metrics
> over an SSH tunnel or a node-local Prometheus exporter.

---

## 7. Production Hardening Checklist

Use this checklist before opening your Portal server to developer traffic:

- [ ] **DNS & TLS**:
  - [ ] Wildcard DNS A record (`*.tunnel.example.com`) resolves to VPS IP.
  - [ ] Valid Let's Encrypt wildcard certificate provisioned via DNS-01 challenge.
  - [ ] TLS 1.3 enforced; legacy SSLv3/TLS 1.0/1.1 disabled.
- [ ] **Access Control**:
  - [ ] Default / blank tokens removed from `/etc/portald/portald.yaml`.
  - [ ] Strong, random tokens generated (`openssl rand -hex 16`).
  - [ ] User quota limits configured (`max_tunnels_per_token: 5-10`).
- [ ] **Firewall & Ports**:
  - [ ] Ports 80, 443, and 8443 open.
  - [ ] Port 9090 verified bound to `127.0.0.1` (`ss -tulpn | grep 9090`).
  - [ ] SSH password authentication disabled (`PasswordAuthentication no`).
- [ ] **Process Isolation**:
  - [ ] `portald` running under dedicated unprivileged systemd user `portald`.
  - [ ] Linux file descriptor limit set to at least 65535 (`LimitNOFILE=65535`).
- [ ] **Telemetry & Health**:
  - [ ] Health endpoint returns HTTP 200: `curl -s http://127.0.0.1:9090/healthz`.
  - [ ] Metrics endpoint scraped securely: `curl -s http://127.0.0.1:9090/metrics`.
  - [ ] `portal doctor` passes all diagnostics from a remote client machine.
