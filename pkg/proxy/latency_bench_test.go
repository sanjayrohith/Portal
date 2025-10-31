package proxy

import (
	"bufio"
	"bytes"
	"net"
	"testing"
	"time"
)

// echoBackend serves a minimal HTTP echo over TCP for burst measurements.
func echoBackend(t *testing.T) (addr string, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no loopback TCP: %v", err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				reader := bufio.NewReader(c)
				for {
					// Read request headers until blank line.
					for {
						line, err := reader.ReadString('\n')
						if err != nil {
							return
						}
						if line == "\r\n" || line == "\n" {
							break
						}
					}
					body := "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: keep-alive\r\n\r\nOK"
					if _, err := c.Write([]byte(body)); err != nil {
						return
					}
				}
			}(conn)
		}
	}()
	return ln.Addr().String(), func() { _ = ln.Close() }
}

func TestBurstLatencyP95P99(t *testing.T) {
	addr, stop := echoBackend(t)
	defer stop()

	const workers = 16
	const perWorker = 10

	summary, err := RunBurst(workers, perWorker, func(_, _ int) error {
		conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
		if err != nil {
			return err
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		req := "GET /burst HTTP/1.1\r\nHost: bench.portal.dev\r\nConnection: close\r\n\r\n"
		if _, err := conn.Write([]byte(req)); err != nil {
			return err
		}
		resp := make([]byte, 256)
		total := 0
		for total < len("HTTP/1.1 200 OK") {
			n, err := conn.Read(resp[total:])
			if err != nil {
				return err
			}
			total += n
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("burst latency: %s", summary)
	if summary.Count != workers*perWorker {
		t.Fatalf("samples = %d, want %d", summary.Count, workers*perWorker)
	}
	if summary.P95 <= 0 || summary.P99 < summary.P95 || summary.Max < summary.P99 {
		t.Fatalf("inconsistent percentiles: %s", summary)
	}
	if summary.P99 > 10*time.Second {
		t.Fatalf("p99 latency %s exceeds budget: %s", summary.P99, summary)
	}
}

func TestLatencyRecorderPercentiles(t *testing.T) {
	recorder := &LatencyRecorder{}
	for i := 1; i <= 100; i++ {
		recorder.Record(time.Duration(i) * time.Millisecond)
	}
	summary := recorder.Summary()
	if summary.P50 != 51*time.Millisecond {
		t.Fatalf("p50 = %s, want 51ms", summary.P50)
	}
	if summary.P95 != 96*time.Millisecond {
		t.Fatalf("p95 = %s, want 96ms", summary.P95)
	}
	if summary.P99 != 100*time.Millisecond {
		t.Fatalf("p99 = %s, want 100ms", summary.P99)
	}
	if empty := (&LatencyRecorder{}).Summary(); empty.Count != 0 {
		t.Fatal("empty recorder should summarize to zero count")
	}
}

func BenchmarkCopyWithSlabThroughput(b *testing.B) {
	payload := make([]byte, SlabSize)
	for i := range payload {
		payload[i] = byte(i)
	}
	// Measure slab copy throughput over pipes.
	b.SetBytes(int64(len(payload)))
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	go func() {
		buf := make([]byte, SlabSize)
		for {
			if _, err := c2.Read(buf); err != nil {
				return
			}
		}
	}()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := CopyWithSlab(c1, bytes.NewReader(payload)); err != nil {
			b.Fatal(err)
		}
	}
}
