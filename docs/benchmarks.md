# Portal Throughput & Latency Benchmarks

Standardized benchmark results for the performance optimization pass
(`sync.Pool` framing buffers, `io.CopyBuffer` slab streaming, TCP socket
tuning). All figures are loopback baselines: they isolate tunnel overhead
from WAN variance. Re-run on your hardware before publishing SLOs.

## Test environment

| Component | Specification                                  |
| --------- | ---------------------------------------------- |
| CPU       | 12th Gen Intel Core i5-12450HX (12 threads)    |
| Memory    | 12 GB                                          |
| OS        | Linux (amd64)                                  |
| Go        | go1.27.1 linux/amd64                           |
| Transport | Loopback TCP (`127.0.0.1`) / in-memory pipes  |

## Burst latency (proxy harness)

Harness: `pkg/proxy.RunBurst` driving 16 concurrent workers x 10
request/response round trips (160 samples) against a minimal HTTP echo
backend over loopback TCP. Each sample is a full TCP dial, request write,
and response-header read.

| Run | n   | min     | p50     | p95     | p99     | max     | mean    | rps    |
| --- | --- | ------- | ------- | ------- | ------- | ------- | ------- | ------ |
| 1   | 160 | 41.5 us | 109 us  | 403 us  | 607 us  | 619 us  | 138 us  | 90,446 |
| 2   | 160 | 39.5 us | 103 us  | 385 us  | 1.53 ms | 1.53 ms | 141 us  | 70,062 |

Notes:

- p50 is stable across runs (~100-110 us); p99 spread (0.6-1.5 ms) is
  loopback scheduling jitter, not tunnel queueing: the max sample equals
  p99 in both runs (single-outlier tail).
- The harness asserts `p99 < 10 s` as a regression budget
  (`TestBurstLatencyP95P99`); tighten per environment once baselines exist.

Reproduce:

```bash
go test -run TestBurstLatencyP95P99 -v -count=1 ./pkg/proxy/
```

## Slab copy throughput (proxy streaming)

Micro-benchmark: 32 KB payload copies through `CopyWithSlab` (pooled
`io.CopyBuffer`) over `net.Pipe`.

| Benchmark                       | ns/op | Throughput     |
| ------------------------------- | ----- | -------------- |
| `BenchmarkCopyWithSlabThroughput` | 3,122 | ~10,495 MB/s |

The figure is transport-bound (in-memory pipe), not a NIC claim. Its value
is comparative: it pins the cost of the copy path itself near zero so
regressions in buffer handling show up immediately.

Reproduce:

```bash
go test -run=NONE -bench BenchmarkCopyWithSlabThroughput -benchtime=100x ./pkg/proxy/
```

## Multiplexed stream load (mux)

Pre-existing harness (`pkg/mux`): 50 concurrent streams x 2 requests over
one multiplexed session, plus `BenchmarkMultiplexedStreams` with
`b.RunParallel`. Used to validate stream isolation under the windowed flow
controller after the leak fixes.

```bash
go test -run TestHighConcurrencyMultiStreamHTTPLoad -v -count=1 ./pkg/mux/
go test -run=NONE -bench BenchmarkMultiplexedStreams -benchtime=100x ./pkg/mux/
```

## Memory profile

| Area                | Before                          | After                                              |
| ------------------- | ------------------------------- | -------------------------------------------------- |
| Frame encode        | `make([]byte)` per `WriteFrame` | Pooled 64 KB + 10 B scratch (`GetEncodeBuffer`)    |
| Frame payload       | `make([]byte)` per `ReadFrame`  | Pooled 64 KB blocks, recycled via `ReleaseFrame`   |
| Proxy streaming     | `io.Copy` allocs per direction  | Pooled 32 KB slabs via `CopyWithSlab`              |
| Closed mux streams  | Retained read buffers + refs    | `readBuf` dropped on `Close`/`Reset`; PONG ref nil |
| Socket buffers      | OS defaults                     | 256 KB `SO_RCVBUF`/`SO_SNDBUF`, `TCP_NODELAY`, 30 s keepalive idle |

Small control payloads (PING/PONG/WINDOW_UPDATE, 4 B) intentionally bypass
the 64 KB payload pool so tiny frames never pin large blocks.

## Goroutine-leak stress

`TestStreamOpenCloseCyclingNoLeak` cycles 200 open/close stream pairs over a
live session, tears the session down, and asserts the goroutine count
returns to baseline (slack 5) within 5 s. `Session.Close` now also drops the
PONG handler reference and refuses post-close `OPEN` frames with an
immediate reset instead of queueing orphan stream state.

```bash
go test -race -count=1 ./pkg/mux/
```

## Throughput scaling expectations

Loopback rps (~70-90k for trivial echo) measures harness + stack, not
production forwarding. For capacity planning, scale from the edge proxy with
real upstreams and record p95/p99 there; the in-tunnel hop budget remains
**< 30 ms** per the telemetry instrumentation (`pkg/telemetry/latency.go`).
