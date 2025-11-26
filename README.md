# Portal

[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.22+-00ADD8.svg)](go.mod)
[![Release](https://img.shields.io/badge/release-v1.0.0-green.svg)](CHANGELOG.md)

**Portal** is an open-source localhost tunneling system designed for developers
who need permanent, stable subdomains for webhook development, mobile API
testing, and remote demos without relying on restrictive third-party services.

> ⚠️ **SECURITY NOTICE: An exposed tunnel is an exposed tunnel.**
> Punching a hole from your local machine to the public internet exposes your
> local server directly to external traffic. Never expose unsecured internal
> administration panels, unauthenticated databases, or unpatched software.
> Always use strong authentication tokens, and consider Portal's built-in HTTP
> Basic Auth or CIDR IP allowlisting middleware when exposing sensitive apps.

---

## Architecture

Portal multiplexes concurrent HTTP/HTTPS transactions over a single, secure,
persistent outbound TLS 1.3 connection initiated from the developer's laptop to
the remote edge server daemon (`portald`):

```
+─────────────────────────────────────────────────────────────────────────────+
|                               PUBLIC INTERNET                               |
|                                                                             |
|   Inbound Webhooks / Mobile Clients / Web Browsers                          |
|                  │ (HTTP/HTTPS :80/:443)                                    |
|                  ▼                                                          |
|        +───────────────────+                                                |
|        |  portald Edge     | ── Extracts subdomain from Host / SNI          |
|        |  Reverse Proxy    | ── Returns branded 404 if inactive             |
|        +─────────┬─────────+                                                |
|                  │ Internal dispatch to active session                      |
|                  ▼                                                          |
|        +───────────────────+                                                |
|        |  Control Plane    | ── SubdomainRegistry (SQLite/Postgres)         |
|        |  Session Manager  | ── Token authentication & rate limits          |
|        +─────────┬─────────+                                                |
+──────────────────┼──────────────────────────────────────────────────────────+
                   │ Persistent Outbound TLS 1.3 Connection (:8443)
                   │ Custom Binary Protocol (10-byte framing + flow control)
+──────────────────┼──────────────────────────────────────────────────────────+
|                  ▼                                                          |
|        +───────────────────+                                                |
|        |  portal Agent     | ── Manages multiplexed logical streams         |
|        |  Demuxer & Bridge | ── Auto-reconnect with exponential backoff     |
|        +────┬─────────┬────+                                                |
|             │         │                                                     |
|             │         ▼                                                     |
|             │  +─────────────────────────+                                  |
|             │  | Web Request Inspector   | ── 127.0.0.1:4040 (Loopback only)|
|             │  | Real-Time SSE Dashboard | ── In-memory ring buffer (200)   |
|             │  | Instant Request Replay  | ── Zero disk persistence         |
|             │  +─────────────────────────+                                  |
|             ▼                                                               |
|   Local Upstream Server (e.g. 127.0.0.1:3000)                               |
|                                                                             |
|                            DEVELOPER WORKSTATION                            |
+─────────────────────────────────────────────────────────────────────────────+
```

---

## Key Features

- **Stable, Persistent Subdomains**: Reclaiming the same custom subdomain
  (`--subdomain myapp`) across reconnects and daemon restarts.
- **Custom Binary Multiplexing**: Purpose-built 10-byte framing protocol with
  sliding-window flow control preventing head-of-line buffer bloat.
- **Local Request Inspector (`127.0.0.1:4040`)**: Modern dark-mode web dashboard
  with real-time SSE updates, JSON syntax formatting, and **Instant Replay**.
- **Automated Wildcard TLS**: Seamless Let's Encrypt wildcard certificate
  issuance via ACME DNS-01 challenges, plus built-in self-signed generators.
- **Built-in Security Controls**: Token bucket rate limiting, per-token tunnel
  quotas, HTTP Basic Authentication, and CIDR IP allowlisting.
- **Zero-Dependency Static Binaries**: Compiles to standalone Go binaries for
  macOS, Linux, and Windows (amd64 and arm64).

---

## Quickstart

### 1. Client Installation

#### Homebrew (macOS / Linux)
```bash
brew tap sanjayrohith/tap
brew install portal
```

#### Debian / Ubuntu (.deb)
```bash
sudo dpkg -i portal_1.0.0_linux_amd64.deb
```

#### From Source (Go 1.22+)
```bash
go install github.com/sanjayrohith/portal/cmd/portal@latest
```

---

### 2. Opening a Tunnel

```bash
# Expose local port 3000 on a random or default subdomain
portal http 3000

# Request a stable custom subdomain
portal http 3000 --subdomain myapp

# Rewrite the Host header to match localhost (for virtual hosts)
portal http 3000 --subdomain myapp --host-header rewrite

# Protect the public endpoint with Basic Authentication
portal http 3000 --subdomain myapp --basic-auth "dev:secret123"

# Specify a custom self-hosted control plane and auth token
portal http 3000 --server tunnel.example.com:8443 --token YOUR_TOKEN --subdomain myapp
```

---

### 3. Diagnostics & Health

Run the diagnostic doctor tool to verify your network environment, DNS
resolution, and TLS handshake:

```bash
portal doctor
```

Inspect live tunnel metrics:

```bash
portal status
```

---

### 4. Running the Server (`portald`)

#### Automated VPS Deployment
```bash
git clone https://github.com/sanjayrohith/portal.git
cd portal
sudo ./deploy.sh --domain tunnel.example.com
```

#### Docker Compose
```bash
docker compose up -d
```

---

## Web Request Inspector (`127.0.0.1:4040`)

Whenever you run `portal http`, an embedded web inspector dashboard starts on
loopback at `http://127.0.0.1:4040`:

- **Real-Time Streaming**: New requests appear instantly via Server-Sent Events.
- **Deep Inspection**: Review HTTP headers, query parameters, timing breakdown,
  and formatted JSON bodies.
- **Instant Replay**: Click **Replay** on any captured request to re-issue it
  directly to your local server without re-triggering the external provider.
- **Zero-Disk Privacy**: Retained exclusively in a 200-request volatile ring
  buffer in RAM. Sensitive webhook tokens are never written to disk.

---

## Standardized Benchmarks

Loopback baseline performance measured on standardized hardware (12th Gen Intel
Core i5-12450HX, Linux amd64, 16 concurrent workers):

| Metric | Measured Value | Target / Note |
| :--- | :--- | :--- |
| **Burst Latency (p50)** | **109 µs** | Ultra-low proxy overhead |
| **Burst Latency (p95)** | **403 µs** | Under 1 ms across 160 sample burst |
| **Burst Latency (p99)** | **607 µs** | Consistent tail latency |
| **Burst Throughput** | **90,446 req/sec** | Fully saturated loopback HTTP |
| **Slab Streaming Copy** | **10,495 MB/s** | `io.CopyBuffer` with preallocated slabs |
| **Framing Overhead** | **10 Bytes / Frame**| Minimal packet header size |

*Full methodology and reproduction commands are documented in [docs/benchmarks.md](docs/benchmarks.md).*

---

## Explicit Limitations (v1.0.0)

Portal is designed with strict boundaries to ensure robustness:

1. **HTTP/HTTPS Only**: Portal v1 is an HTTP reverse-proxy tunnel. Raw TCP or
   UDP transport tunneling (e.g. raw SSH, Minecraft, or external PostgreSQL
   port forwarding) is explicitly **not supported** in v1.
2. **Volatile Inspector Storage**: The request inspector buffer lives in memory
   only. Restarting `portal` resets the inspector history. Payloads exceeding
   ring buffer limits are evicted in FIFO order.
3. **Subdomain Constraints**: Subdomains must conform strictly to RFC 1123 DNS
   label specifications (alphanumeric and hyphens, 1-63 characters).
   System-reserved names (`api`, `admin`, `portal`, `mail`, `metrics`) cannot
   be registered.
4. **Single-Node Default**: The default standalone VPS setup uses SQLite.
   Multi-instance distributed clustering requires configuring the PostgreSQL
   storage driver.

---

## Documentation & Guides

- [Transport Protocol Architecture](docs/architecture.md) — In-depth binary wire format and flow control rationale
- [Self-Hosting Guide](docs/self-hosting.md) — Complete VPS, DNS, and TLS setup instructions
- [Security Hardening Checklist](docs/security.md) — Production deployment security checklist
- [Throughput & Latency Benchmarks](docs/benchmarks.md) — Performance metrics and reproducibility guides
- [Webhook Integration Examples](examples/README.md) — Runnable Stripe, GitHub, and Slack receiver fixtures

---

## License

Apache License 2.0. See [LICENSE](LICENSE) for details.
