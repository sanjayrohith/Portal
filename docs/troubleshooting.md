# Production Troubleshooting Guide & Operational FAQ

This guide provides operational triage procedures, diagnostic command sequences,
and answers to frequently asked questions for operators running the **Portal**
server daemon (`portald`) and developers using the client agent (`portal`).

---

## 1. Quick Triage Flowchart

When diagnosing a broken or degraded tunnel, follow this triage order:

```
                  ┌──────────────────────────────┐
                  │ Run: portal doctor           │
                  └──────────────┬───────────────┘
                                 │
                 ┌───────────────┴───────────────┐
                 ▼                               ▼
          Checks Pass                       Check Fails
                 │                               │
        ┌────────┴────────┐             ┌────────┴────────┐
        ▼                 ▼             ▼                 ▼
   Target Local     Edge / Public   DNS/Network      TLS / Auth
     Upstream           HTTP          Failed           Failed
        │                 │             │                 │
    Section 5         Section 5     Section 2         Section 3
```

---

## 2. Automated Diagnostics with `portal doctor`

Before manual debugging, run the built-in diagnostic probe:

```bash
portal doctor --server tunnel.example.com:8443 --target localhost:3000
```

To export results for bug reports or automated CI monitoring:

```bash
portal doctor --json
```

---

## 3. Network & Transport Triage

### 3.1 Connection Refused on Port 8443

**Symptoms**: `dial tcp tunnel.example.com:8443: connect: connection refused`

**Root Causes & Remedies**:
1. **Server Daemon Inactive**:
   ```bash
   sudo systemctl status portald
   # If inactive, inspect logs:
   sudo journalctl -u portald -n 50 --no-pager
   ```
2. **Firewall Blocking Port 8443**:
   ```bash
   # On VPS:
   sudo ufw status | grep 8443
   sudo ufw allow 8443/tcp comment "Portal Control Plane"
   ```
3. **Cloud Provider Security Group**: Ensure your cloud provider (AWS EC2,
   Hetzner, DigitalOcean) security group allows inbound TCP on port 8443.

---

### 3.2 Wi-Fi to Ethernet Network Interface Handoffs

**Symptoms**: Client laptop moves from Wi-Fi to Ethernet dock (or wakes from
sleep); terminal hangs without receiving requests or reconnecting.

**Resolution**:
- Portal includes `TCP_USER_TIMEOUT` (10s) socket tuning and interface
  migration detection in `HealthWatchdog`.
- If an interface migration occurs, Portal tears down the severed connection and
  initiates an immediate exponential-backoff reconnect.
- Ensure your client is running Portal v1.0.0+ with default keepalive settings.
- Check live watchdog status:
  ```bash
  portal status
  ```

---

### 3.3 Corporate Proxies & Deep Packet Inspection (DPI)

**Symptoms**: TLS handshake starts but hangs or drops with `connection reset by peer`.

**Remedy**:
Corporate firewalls often block non-HTTP TLS traffic on unusual ports like 8443.
- If port 8443 is blocked by a corporate perimeter firewall, configure `portald`
  to accept control connections via a reverse proxy terminating on port 443 with
  SNI routing (e.g. `control.tunnel.example.com:443`).

---

## 4. DNS & Wildcard Domain Resolution

### 4.1 Subdomain Fails to Resolve (`NXDOMAIN` or `SERVFAIL`)

**Symptoms**: `curl https://myapp.tunnel.example.com` fails with DNS errors.

**Triage Steps**:
1. **Verify Wildcard A Record**:
   ```bash
   dig +short *.tunnel.example.com
   # Must return your VPS public IP address
   ```
2. **Check Authoritative Nameservers**:
   ```bash
   dig +trace myapp.tunnel.example.com
   ```
3. **Local DNS Cache Flush**:
   - macOS: `sudo dscacheutil -flushcache; sudo killall -HUP mDNSResponder`
   - Linux: `sudo resolvectl flush-caches`

---

## 5. TLS & Certificate Issues

### 5.1 ACME DNS-01 Challenge Failures

**Symptoms**: Server daemon logs: `acme: error presenting token: ...`

**Remedies**:
- Wildcard certificates (`*.tunnel.example.com`) strictly require **DNS-01**
  challenges; HTTP-01 challenges cannot issue wildcards.
- Verify provider API credentials in `/etc/portald/portald.yaml`.
- Ensure DNS provider API tokens have `Zone:DNS:Edit` permissions.
- Ensure the server has permission to write to `acme_dir` (e.g. `/var/lib/portald/acme`).

### 5.2 Self-Signed Certificate Errors

**Symptoms**: `x509: certificate signed by unknown authority`

**Remedy**:
For local development or offline staging:
```bash
portal http 3000 --insecure
```
> ⚠️ **Warning**: Never use `--insecure` in production. Distribute the generated
> root CA or provision valid ACME certificates.

---

## 6. Upstream & HTTP Forwarding Issues

### 6.1 Branded 404 Fallback Page Returned

**Symptoms**: Visiting `https://myapp.tunnel.example.com` displays Portal's
branded 404 page ("Tunnel Not Found or Inactive").

**Cause**:
The requested subdomain is valid DNS-wise, but no active client agent is
currently connected to `portald` holding that claim.

**Triage Steps**:
1. Verify client agent is running:
   ```bash
   portal status
   ```
2. Verify subdomain claim in server registry:
   ```bash
   sudo portald tunnels list --admin-addr 127.0.0.1:9090
   ```
3. Check if token expired or client was disconnected due to network disruption.

---

## 6.2 502 Bad Gateway / Connection Refused to Upstream

**Symptoms**: Browser returns `502 Bad Gateway`. Client logs display:
`dial tcp 127.0.0.1:3000: connect: connection refused`.

**Cause**:
The local application server is not running or is bound to a different port or IP family.

**Remedies**:
1. Ensure your local app is listening:
   ```bash
   curl -I http://127.0.0.1:3000
   ```
2. **IPv4 vs IPv6 Loopback Mismatch**:
   Some dev servers (e.g. Next.js, Node.js 18+) bind to `localhost` which
   resolves to `[::1]` (IPv6). If your upstream only listens on IPv6:
   ```bash
   portal http "[::1]:3000" --subdomain myapp
   ```

---

### 6.3 Virtual Host & Host Header Rewriting

**Symptoms**: Local dev server returns `404 Not Found` or `Invalid Host Header`.

**Cause**:
The dev server (e.g. Webpack Dev Server, Django `ALLOWED_HOSTS`) rejects
incoming requests because the `Host` header is `myapp.tunnel.example.com`.

**Remedy**:
Use the host-header rewrite flag:
```bash
portal http 3000 --subdomain myapp --host-header rewrite
```
This replaces `Host: myapp.tunnel.example.com` with `Host: localhost:3000`.

---

### 6.4 Webhook Signature Verification Fails

**Symptoms**: Stripe or GitHub webhook fails with `401 Unauthorized` or `Invalid Signature`.

**Causes & Remedies**:
1. **Raw Body Tampering**: Ensure your local web framework does not parse or
   modify the raw request body before signature checking (e.g. in Express, use
   `express.raw({ type: 'application/json' })`).
2. **Replay Tolerance**: Webhook providers enforce timestamp tolerances (e.g. 5
   minutes). If using Portal's web inspector **Replay** feature, disable
   timestamp tolerance during local development.

---

## 7. Web Inspector Triage (`127.0.0.1:4040`)

### 7.1 Port Already in Use

**Symptoms**: `failed to bind inspector on 127.0.0.1:4040: address already in use`

**Remedy**:
Specify an alternative loopback port:
```bash
portal http 3000 --inspector-addr 127.0.0.1:4041
```

### 7.2 Strict Loopback Security Violation

**Symptoms**: `fatal: inspector address must bind strictly to 127.0.0.1 or localhost`

**Cause**:
For security compliance, Portal categorically rejects binding the web inspector
to public interfaces (`0.0.0.0` or external LAN IPs) to prevent secret leakage.

---

## 8. Frequently Asked Questions (FAQ)

### Q: Can I run multiple tunnels from the same machine simultaneously?
**A**: Yes. Run each in a separate terminal with distinct local ports and
subdomains:
```bash
portal http 3000 --subdomain frontend --inspector-addr 127.0.0.1:4040
portal http 8080 --subdomain backend  --inspector-addr 127.0.0.1:4041
```

### Q: Does Portal support raw TCP tunnels (SSH, Postgres, Minecraft)?
**A**: Portal v1.0.0 is an HTTP/HTTPS reverse-proxy tunnel. Raw TCP/UDP port
forwarding is deliberately out of scope for v1 to maintain a minimalist footprint
and zero-dependency streaming architecture.

### Q: How do I retain my subdomain if my laptop reboots?
**A**: Portal control plane includes persistent storage (SQLite or PostgreSQL)
and subdomain reclamation. As long as you reuse your authentication token and
request the same subdomain (`--subdomain myapp`), the control plane reclaims
your reservation.

### Q: Where are inspector request bodies stored on disk?
**A**: Nowhere. Portal's request inspector retains transactions exclusively in an
in-memory circular ring buffer in RAM (default: 200 items). No request bodies,
cookies, or auth headers are ever written to disk.

### Q: How do I update server tokens without dropping tunnels?
**A**: Edit `/etc/portald/portald.yaml` and send SIGHUP:
```bash
sudo systemctl reload portald
```
Existing multiplexed tunnels remain completely uninterrupted.
