# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [1.0.0] - 2025-11-26

### Initial Release

Portal is an open-source localhost tunneling system designed for developers who
require permanent, stable subdomains for webhook development, mobile API
testing, and remote demos without depending on restrictive third-party services.

### Added

- **Custom Binary Transport Protocol (`pkg/protocol`, `pkg/mux`)**:
  - Purpose-built 10-byte fixed header schema (`Type`, `Flags`, `StreamID`, `Length`).
  - Frame types: `DATA`, `OPEN`, `CLOSE`, `PING`, `PONG`, and `WINDOW_UPDATE`.
  - Stream ID parity partitioning: Odd stream IDs initiated by client, even by server.
  - Full stream lifecycle state machine implementing Go `net.Conn` interface.
  - Per-stream sliding window flow control (256 KB initial window, 128 KB batch replenishment) preventing head-of-line buffer bloat.
  - Heartbeat keepalive with sub-millisecond round-trip time (RTT) tracking.
- **Control Plane & Subdomain Registry (`pkg/registry`, `pkg/control`, `cmd/portald`)**:
  - Thread-safe in-memory SubdomainRegistry with RFC 1123 label validation and system reserved names filtering.
  - Persistent storage backends for SQLite (VPS single-node) and PostgreSQL (HA cluster).
  - Subdomain reclamation handshake allowing reconnecting clients to immediately retain previously held subdomains.
  - Administrative token management CLI suite (`portald tokens create/revoke/list`).
  - SIGHUP signal handler for hot-reloading configurations, tokens, and rate limits without dropping connections.
- **Edge Reverse Proxy (`pkg/edge`, `pkg/proxy`)**:
  - Public HTTP/HTTPS listeners on ports 80 and 443.
  - Virtual host router extracting subdomains from HTTP `Host` headers and TLS SNI extensions.
  - Host header rewrite engine (`--host-header rewrite`) for upstream virtual hosts.
  - Asynchronous stream worker pool with non-blocking local upstream dispatching.
  - Branded HTML 404 fallback page for unallocated or disconnected subdomains.
- **Authentication & Security (`pkg/auth`, `pkg/security`)**:
  - Mandatory token authentication challenge during TLS control handshake.
  - Constant-time token verification eliminating timing side-channel attacks.
  - Per-token rate limiting using token bucket algorithms.
  - Concurrency stream quotas, request payload size caps, and header boundaries.
  - Optional HTTP Basic Authentication and CIDR IP allowlisting middleware.
- **Web Request Inspector UI (`pkg/inspector`)**:
  - Dedicated local HTTP server strictly bound to `127.0.0.1:4040` (explicitly refusing external network interfaces).
  - In-memory circular ring buffer (default 200 requests) with zero disk persistence to ensure secret safety.
  - Real-time live request streaming via Server-Sent Events (SSE).
  - Dark-mode dashboard with header parsing, query parameter viewer, JSON syntax formatting, and timing breakdowns.
  - Interactive **Replay** button to re-issue captured HTTP requests directly against localhost.
  - Embedded static web assets using `go:embed` for single-binary zero-dependency distribution.
- **Automated TLS & Certificates (`pkg/transport`)**:
  - TLS 1.3 enforced transport with ALPN `portal/1` negotiation.
  - Automated Let's Encrypt wildcard certificates (`*.yourdomain.com`) via ACME DNS-01 challenge automation.
  - Self-signed wildcard certificate generator utility for offline and local staging.
- **Telemetry & Diagnostics (`pkg/telemetry`, `pkg/doctor`)**:
  - Prometheus metrics collector exposing active tunnels, streams, bytes, and latency histograms on `/metrics`.
  - Health and readiness probe endpoints `/healthz` and `/readyz`.
  - Interactive terminal status banner rendering session statistics and public URLs.
  - `portal status` CLI command querying live agent statistics.
  - `portal doctor` diagnostic tool probing loopback networking, inspector port availability, DNS resolution, TCP connectivity, and TLS handshake.
- **Deployment & Packaging**:
  - Automated VPS deployment script (`deploy.sh`) configuring systemd service, directories, and UFW firewall.
  - Production `docker-compose.yml` environment with mock DNS and echo upstream.
  - Multi-stage unprivileged Dockerfiles for `portal` and `portald`.
  - Packaging specifications for Homebrew Formula (`packaging/homebrew/portal.rb`), Debian (`packaging/debian/`), RPM (`packaging/rpm/portal.spec`), and NFPM (`packaging/nfpm.yaml`).
  - GitHub Actions CI pipeline running lint, unit tests, race detector, and containerized E2E integration suites.

### Performance

- Zero-allocation hot path using `sync.Pool` for frame encoding scratch buffers and payload recycling.
- Streaming I/O forwarding with preallocated slab buffers via `io.CopyBuffer`.
- TCP socket optimization with `TCP_NODELAY` and customized keepalive intervals.
- Benchmarked at over 10,000 MB/s streaming throughput and sub-150µs p50 burst latency over loopback.
