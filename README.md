# Portal

Portal is an open-source localhost tunneling tool designed for developers who need permanent, stable subdomains for webhook development, mobile API testing, and remote demos without relying on restrictive third-party services.

## Architecture

```
Internet → :443 Edge proxy ──┐
                             │ (control plane: subdomain → conn registry)
                             │
                    persistent outbound conn
                             │
                       Client agent (laptop)
                             │
                    localhost:3000 (your app)
```

The client initiates an outbound TLS connection to the remote edge server, eliminating the need for open inbound ports, router port-forwarding, or public IP addresses on the developer machine.

## Key Features

- **Stable, user-chosen subdomains**: Claim a persistent subdomain that stays yours across reconnects and daemon restarts.
- **NAT & Firewall Traversal**: Operates strictly via outbound connections.
- **Local Request Inspector**: In-memory inspection dashboard running at `127.0.0.1:4040` with instant request replay.
- **Stream Multiplexing**: Concurrent HTTP/HTTPS requests over a single persistent TCP tunnel with custom binary framing and windowed flow control.
- **Zero-Dependency Static Binaries**: Distributed as standalone Go binaries for client (`portal`) and server (`portald`).
