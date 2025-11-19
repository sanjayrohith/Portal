# Transport Protocol Architecture: Framing, Multiplexing & Flow Control

This document details the architectural design and operational rationale of the
custom binary transport protocol used by **Portal**. It describes the wire
framing specification, stream multiplexing state machine, sliding window flow
control mechanics, keepalive diagnostics, and memory recycling strategies.

---

## 1. Architectural Motivation & Rationale

Localhost tunneling systems must route hundreds of distinct, concurrent HTTP
transactions across a single outbound TCP/TLS connection established between the
client agent (on a developer workstation) and the edge control plane daemon.

### The Transport Dilemma: Path A vs. Path B

When architecting a multiplexed tunnel transport, two primary paths exist:

```
                  ┌──────────────────────────────────────────────┐
                  │          Transport Architecture Path         │
                  └───────┬──────────────────────────────┬───────┘
                          │                              │
             Path A: Standard Layer          Path B: Custom Framing
             (HTTP/2, gRPC, Yamux)             (Portal Protocol)
                          │                              │
         • Ready-made libraries          • Purpose-built 10-byte header
         • Heavy protocol overhead       • Zero third-party runtime deps
         • Limited control over frames   • Fine-grained stream backpressure
         • Strict framing constraints    • Direct sync.Pool byte recycling
```

- **Path A (HTTP/2 or Yamux)**: Off-the-shelf multiplexing libraries provide
  battle-tested connection pooling and framing. However, they carry significant
  framing bloat (e.g., HPACK dynamic tables, complex priority trees), obscure
  transport internals, and limit fine-grained control over buffer reuse and
  low-level connection lifecycle events.
- **Path B (Portal Custom Binary Framing)**: Portal implements a purpose-built,
  minimalist binary framing protocol. By constraining frame headers to a fixed
  10-byte schema and decoupling flow control from application-level HTTP
  semantics, the protocol achieves:
  1. **Minimal Framing Overhead**: Only 10 bytes of header per data chunk.
  2. **Predictable Backpressure**: Strict per-stream sliding windows prevent a
     single slow local backend from starving unrelated concurrent streams.
  3. **Zero-Allocation Buffer Recycling**: Exact payload sizes allow direct
     integration with `sync.Pool` slab allocators.
  4. **Transparent Proxying**: Unaltered transit of raw HTTP/1.1 chunks,
     Server-Sent Events (SSE), and bidirectional byte streams.

---

## 2. End-to-End System Topology

The transport protocol operates exclusively across the persistent outbound TLS
connection between the developer agent (`portal`) and the server daemon
(`portald`):

```
+-----------------------------------------------------------------------------+
|                                PUBLIC INTERNET                              |
|                                                                             |
|   Inbound Webhook / User Browser                                            |
|                  │ (HTTP/HTTPS :80/:443)                                    |
|                  ▼                                                          |
|        +───────────────────+                                                |
|        |  portald Edge     |                                                |
|        |  Reverse Proxy    |                                                |
|        +─────────┬─────────+                                                |
|                  │ Host router: subdomain -> session lookup                 |
|                  ▼                                                          |
|        +───────────────────+                                                |
|        |  Control Plane    |                                                |
|        |  Session Manager  |                                                |
|        +─────────┬─────────+                                                |
+──────────────────┼──────────────────────────────────────────────────────────+
                   │ Custom Binary Frame Protocol
                   │ (Outbound TLS :8443, Mux Session)
+──────────────────┼──────────────────────────────────────────────────────────+
|                  ▼                                                          |
|        +───────────────────+                                                |
|        |  portal Agent     |                                                |
|        |  Mux Demuxer      |                                                |
|        +─────────┬─────────+                                                |
|                  │ Dispatch to local worker pool                            |
|                  ▼                                                          |
|   Local Upstream Target (e.g. 127.0.0.1:3000)                               |
|                                                                             |
|                            DEVELOPER WORKSTATION                            |
+-----------------------------------------------------------------------------+
```

---

## 3. Binary Frame Specification (Wire Format)

Every message transmitted over the multiplexed transport consists of a fixed
10-byte header followed by a variable-length payload. Numbers are encoded in
network byte order (Big-Endian).

### Header Layout

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|     Type      |     Flags     |          Stream ID            |
|    (8-bit)    |    (8-bit)    |           (32-bit)            |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|          Stream ID (cont.)    |            Length             |
|                               |           (32-bit)            |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|          Length (cont.)       |    Payload (Length bytes)...  |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+                               +
|                                                               |
+---------------------------------------------------------------+
```

### Field Breakdown

| Offset | Field | Type | Description |
| :--- | :--- | :--- | :--- |
| `0x00` | `Type` | `uint8` | Identifies frame semantics (`DATA`, `OPEN`, etc.). |
| `0x01` | `Flags` | `uint8` | Bitfield flags modifying frame behavior (`FIN`, `RST`, `ACK`). |
| `0x02-0x05` | `StreamID` | `uint32` | Identifier for the logical multiplexed stream. |
| `0x06-0x09` | `Length` | `uint32` | Byte length of the following payload (`0` to `65536`). |
| `0x0A...` | `Payload` | `[]byte` | Raw payload bytes conforming to `Length`. |

### Frame Types

Portal defines six discrete frame types:

1. **`DATA` (`0x01`)**: Carries stream data bytes. May assert `FlagFin` to
   indicate stream half-close.
2. **`OPEN` (`0x02`)**: Signals the initiation of a new logical stream. Payload
   length is typically 0.
3. **`CLOSE` (`0x03`)**: Signals the teardown of a stream. If flagged with
   `FlagRst`, causes immediate reset; otherwise indicates graceful half-close.
4. **`PING` (`0x04`)**: Heartbeat frame carrying a 4-byte opaque nonce payload
   used for dead-connection detection and round-trip latency tracking.
5. **`PONG` (`0x05`)**: Response to a `PING` frame, echoing the exact 4-byte
   nonce payload.
6. **`WINDOW_UPDATE` (`0x06`)**: Conveys flow control credit replenishment.
   Carries a 4-byte uint32 payload indicating the number of additional bytes
   the remote sender is allowed to transmit on that `StreamID`.

### Flags

| Flag Constant | Bit Value | Semantics |
| :--- | :--- | :--- |
| `FlagNone` | `0x00` | Default state; no modifiers. |
| `FlagFin` | `0x01` | Half-close indicator. Signals no further data will be sent by this peer on this stream. |
| `FlagRst` | `0x02` | Abrupt reset indicator. Aborts the stream immediately and discards unread buffers. |
| `FlagAck` | `0x04` | Reserved for handshake and control acknowledgments. |

---

## 4. Stream Multiplexing Architecture

A `Session` wraps a single underlying `net.Conn` (usually TLS 1.3 over TCP) and
exposes a multi-stream abstraction where each `Stream` implements `net.Conn`.

### Stream ID Allocation & Parity Partitioning

To avoid coordination roundtrips when allocating stream identifiers, Portal uses
numeric parity partitioning:

- **Client-Initiated Streams**: Strictly **odd** identifiers (`1, 3, 5, 7, ...`).
- **Server-Initiated Streams**: Strictly **even** identifiers (`2, 4, 6, 8, ...`).

In the standard tunnel topology, incoming public HTTP requests arriving at
`portald` prompt the server to initiate streams to the client agent using even
stream IDs. Client-side reverse dials or management probes use odd stream IDs.

### Stream Lifecycle State Machine

Each logical stream progresses through well-defined lifecycle states:

```
                    ┌──────────────┐
                    │     INIT     │
                    └──────┬───────┘
                           │ OpenStream() or incoming FrameOpen
                           ▼
                    ┌──────────────┐
     ┌─────────────►│     OPEN     │◄─────────────┐
     │              └──────┬───────┘              │
     │ Local Close()       │                      │ Remote FIN
     │ (sends FIN)         │ FrameClose(RST)      │ (receives FIN)
     ▼                     │ or stream error      ▼
┌──────────────────┐       │             ┌──────────────────┐
│ HALF_CLOSED_LOCAL│       │             │HALF_CLOSED_REMOTE│
└────────┬─────────┘       │             └────────┬─────────┘
         │ Remote FIN      ▼                      │ Local Close()
         │             ┌─────────┐                │ (sends FIN)
         └────────────►│  RESET  │◄───────────────┘
                       └────┬────┘
                            │
                            ▼
                       ┌─────────┐
                       │ CLOSED  │
                       └─────────┘
```

#### State Definitions

- **`INIT`**: Initial unallocated stream state.
- **`OPEN`**: Bidirectional communication active. Both peers may read and write
  data frames according to flow control credit.
- **`HALF_CLOSED_LOCAL`**: Local endpoint called `Close()`. A `DATA` frame with
  `FlagFin` (or empty `CLOSE`) was dispatched. The local endpoint can no longer
  write, but can continue reading responses until remote EOF.
- **`HALF_CLOSED_REMOTE`**: Remote endpoint transmitted `FlagFin`. Local reads
  return `io.EOF` once buffered data is exhausted. Local writes remain permitted
  until local close.
- **`CLOSED`**: Both transmission directions completed gracefully. Stream is
  unregistered from the session map.
- **`RESET`**: Abnormal termination triggered by `FlagRst`. Read buffers are
  cleared and pending read/write operations unblock with an error.

### Demultiplexing & Dispatch (`recvLoop`)

A single dedicated goroutine per `Session` executes `recvLoop()`, continuously
decoding binary frames from the network reader:

```go
func (s *Session) handleIncomingFrame(f *protocol.Frame) {
    switch f.Type {
    case protocol.FrameOpen:
        // Instantiate stream, register in map, enqueue in acceptCh (cap 256)
    case protocol.FrameData:
        // Lookup stream, push payload into stream read buffer, evaluate FlagFin
    case protocol.FrameWindowUpdate:
        // Extract 4-byte uint32 credit, unblock waiting writers
    case protocol.FrameClose:
        // Handle graceful EOF (FlagFin) or abrupt abort (FlagRst)
    case protocol.FramePing:
        // Immediately reply with FramePong echoing nonce
    case protocol.FramePong:
        // Dispatch nonce to ping manager to complete RTT latency calculation
    }
}
```

---

## 5. Windowed Flow Control & Backpressure Rationale

The central challenge in transport multiplexing is **Head-of-Line (HoL) buffer
bloat and cross-stream starvation**.

### The Starvation Problem

Consider two HTTP requests multiplexed over a single TCP connection:
1. **Stream A**: Downloading a 500 MB static binary to a local disk or slow consumer.
2. **Stream B**: A critical, lightweight webhook notification (`POST /webhook/stripe`).

If flow control only operates at the TCP layer, the TCP receive window reflects
the aggregate buffer state of the whole connection. If Stream A's local consumer
is slow, unread data from Stream A fills the TCP socket buffer. The OS shrinks the
TCP window to zero. As a result, **Stream B cannot receive any packets**, even
though Stream B's handler is completely idle and ready to process data.

```
Without Stream Flow Control:
TCP Window: [ Stream A Data | Stream A Data | Stream A Data | Stream A Data ] (Full!)
Stream B Packet: [ DROP / BLOCKED AT OS ] -> Stream B Starved!
```

### The Solution: Per-Stream Sliding Window Credit

Portal decouples transport throughput from individual stream consumption by
enforcing per-stream sliding window credits:

- **Initial Window Size (`DefaultInitialWindowSize`)**: `256 KB` (262,144 bytes).
- **Replenishment Threshold (`DefaultReplenishThreshold`)**: `128 KB`
  (`InitialWindow / 2`).

```
                Sender Window                                Receiver Window
             ┌─────────────────┐                          ┌─────────────────┐
             │ Send Credit:    │                          │ Capacity:       │
             │     256 KB      │                          │     256 KB      │
             └────────┬────────┘                          └────────┬────────┘
                      │                                            │
                      │  DATA (64 KB)                              │
                      ├───────────────────────────────────────────►│ [Buf: 64 KB]
                      │  Send Credit: 192 KB                       │
                      │                                            │
                      │  DATA (64 KB)                              │
                      ├───────────────────────────────────────────►│ [Buf: 128 KB]
                      │  Send Credit: 128 KB                       │
                      │                                            │ (App reads 128KB)
                      │                                            │ (Consumed >= 128KB)
                      │  WINDOW_UPDATE (+128 KB)                   │
                      │◄───────────────────────────────────────────┤
                      │  Send Credit: 256 KB                       │
```

#### Outbound Sender Flow Control (`acquireSendCredit`)

Before transmitting any `DATA` frame:
1. The writer calculates `min(len(payload), availableSendCredit, DefaultMaxPayloadLength)`.
2. If `sendWindow == 0`, the writer goroutine blocks on `sync.Cond` until a
   `WINDOW_UPDATE` arrives or the stream is closed.
3. Once credit is available, `sendWindow` is decremented by the payload size,
   and the `DATA` frame is dispatched atomically to the connection.

#### Inbound Receiver Flow Control (`notifyConsumed`)

1. Incoming `DATA` frames deposit bytes directly into the stream's circular read
   buffer.
2. When the consumer application executes `Read()`, bytes leave the buffer.
3. The stream increments `unreplenishedBytes` by the number of bytes read.
4. When `unreplenishedBytes >= threshold` (128 KB), a `WINDOW_UPDATE` frame
   carrying the credit delta is transmitted to the remote peer, and
   `unreplenishedBytes` resets to 0.

#### Why Batch Replenishment?

Sending a `WINDOW_UPDATE` for every small read (e.g. 1 KB) introduces "silly
window syndrome" and inundates the underlying TLS connection with framing
overhead (10-byte header + 4-byte payload per read). By deferring updates until
at least 50% of the window is consumed, Portal guarantees high pipeline
efficiency without risking sender stalls.

---

## 6. Heartbeat & Keepalive Protocol

Silent TCP disconnections (e.g. stateful NAT router eviction, laptop sleep,
cellular handoff) can leave half-open sockets lingering indefinitely on the
server.

Portal implements an active probing heartbeat subsystem (`pingManager`):

```
Client Agent                                                Server Daemon
     │                                                            │
     ├─────────── FramePing (StreamID: 0, Nonce: 0x1A2B3C4D) ─────►│
     │                                                            │
     │◄────────── FramePong (StreamID: 0, Nonce: 0x1A2B3C4D) ─────┤
     │                                                            │
     RTT = Now() - PingStartTime
```

### Protocol Invariants for Keepalive

1. **Reserved Stream ID**: All `PING` and `PONG` frames use `StreamID = 0`.
2. **Nonce Verification**: A 4-byte cryptographically random or monotonically
   incrementing nonce is packed into the payload. The responder must echo the
   exact nonce back in the `PONG` payload.
3. **Round-Trip Latency (RTT)**: Latency is computed upon receipt of the
   matching nonce. The metric is exposed to client status banners and
   Prometheus telemetry histograms (`portal_tunnel_hop_latency_seconds`).
4. **Dead-Connection Eviction**: If a configurable threshold of consecutive
   pings fail (e.g. 3 missed pings over 30 seconds), the session terminates
   abruptly, triggering the client's exponential-backoff reconnect supervisor.

---

## 7. Memory Management & Zero-Allocation Hot Path

To sustain high throughput (10,000+ MB/s over loopback, 90,000+ RPS) without
triggering Go Garbage Collector (GC) stalls, the transport protocol employs
pooling patterns in `pkg/protocol/pool.go`:

### Write Buffer Recycling (`encodeBufPool`)

Serializing a frame requires assembling the 10-byte header and payload into a
contiguous slice. Instead of allocating `make([]byte, 10 + len(payload))` on
every frame:
- An internal `sync.Pool` manages scratch buffers preallocated to
  `HeaderSize + DefaultMaxPayloadLength` (65,546 bytes).
- `GetEncodeBuffer(n)` leases a scratch slice.
- `PutEncodeBuffer(buf)` recycles the slice immediately once `w.Write()`
  completes. Write scratch buffers never escape the calling scope.

### Read Payload Recycling (`payloadPool`)

When reading incoming frames:
- Control frames (<= 4 bytes) use inline memory to avoid pinning 64 KB pool
  blocks.
- Data payloads lease a 64 KB buffer from `payloadPool`.
- Downstream proxy forwarders return the buffer to the pool via
  `protocol.ReleaseFrame(frame)` once the payload has been written to the local
  upstream TCP socket.

### Socket Configuration

The underlying TCP connection is tuned upon establishment:
- `TCP_NODELAY = true`: Disables Nagle's algorithm to eliminate 40 ms delayed
  ACK stalls on small control and header frames.
- `KeepAlive = 15s`: Enables OS-level TCP keepalive probes to assist the
  application-level PING/PONG watchdog.

---

## 8. Summary of Protocol Invariants

| Parameter | Value | Rationale |
| :--- | :--- | :--- |
| **Header Size** | Fixed 10 Bytes | Minimal overhead; zero ambiguity in parsing. |
| **Max Payload Size** | 64 KB (`65536` bytes) | Matches standard TCP window chunks and bounds memory allocation. |
| **Initial Stream Window**| 256 KB (`262144` bytes)| Sufficient bandwidth-delay product for WAN links without excessive buffering. |
| **Replenishment Threshold**| 128 KB (`131072` bytes)| Minimizes control frame churn while guaranteeing uninterrupted streaming. |
| **Client Stream Parity** | Odd (`1, 3, 5, ...`) | Prevents stream ID collision without synchronization roundtrips. |
| **Server Stream Parity** | Even (`2, 4, 6, ...`) | Dedicated space for incoming public request dispatches. |
| **Stream ID 0** | Session Control | Exclusively reserved for session-level control, `PING`, and `PONG`. |
| **Accept Queue Capacity**| 256 pending streams | Bounds memory under sudden connection spikes; rejects excess with `RST`. |
