package mux

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

func TestHighConcurrencyMultiStreamHTTPLoad(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	serverSession := NewSession(c1, true)
	clientSession := NewSession(c2, false)
	defer serverSession.Close()
	defer clientSession.Close()

	const concurrentStreams = 50
	const requestsPerWorker = 2

	// Server-side responder echoing mock HTTP responses for each opened stream
	go func() {
		for {
			stream, err := serverSession.AcceptStream()
			if err != nil {
				return
			}

			go func(st *Stream) {
				defer st.Close()

				// Read dummy HTTP request line & headers until \r\n\r\n
				buf := make([]byte, 512)
				_, readErr := st.Read(buf)
				if readErr != nil && readErr != io.EOF {
					return
				}

				body := fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: 15\r\n\r\nstream-%d-done", st.ID())
				_, _ = st.Write([]byte(body))
			}(stream)
		}
	}()

	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(concurrentStreams)

	for i := 0; i < concurrentStreams; i++ {
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < requestsPerWorker; j++ {
				stream, err := clientSession.OpenStream()
				if err != nil {
					t.Errorf("worker %d req %d failed to open stream: %v", workerID, j, err)
					return
				}

				req := fmt.Sprintf("GET /bench/%d/%d HTTP/1.1\r\nHost: bench.portal.dev\r\n\r\n", workerID, j)
				if _, err := stream.Write([]byte(req)); err != nil {
					t.Errorf("worker %d req %d write failed: %v", workerID, j, err)
					_ = stream.Close()
					return
				}

				respBuf := make([]byte, 512)
				n, err := stream.Read(respBuf)
				if err != nil && err != io.EOF {
					t.Errorf("worker %d req %d read failed: %v", workerID, j, err)
					_ = stream.Close()
					return
				}

				if !bytes.Contains(respBuf[:n], []byte("200 OK")) {
					t.Errorf("worker %d req %d unexpected response: %s", workerID, j, string(respBuf[:n]))
				}

				_ = stream.Close()
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)

	totalRequests := concurrentStreams * requestsPerWorker
	rps := float64(totalRequests) / duration.Seconds()
	t.Logf("Completed %d streams in %v (%.2f req/s)", totalRequests, duration, rps)
}

func BenchmarkMultiplexedStreams(b *testing.B) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	serverSession := NewSession(c1, true)
	clientSession := NewSession(c2, false)
	defer serverSession.Close()
	defer clientSession.Close()

	go func() {
		for {
			stream, err := serverSession.AcceptStream()
			if err != nil {
				return
			}
			go func(st *Stream) {
				defer st.Close()
				buf := make([]byte, 128)
				_, _ = st.Read(buf)
				_, _ = st.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK"))
			}(stream)
		}
	}()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			st, err := clientSession.OpenStream()
			if err != nil {
				b.Errorf("OpenStream error: %v", err)
				return
			}
			_, _ = st.Write([]byte("GET / HTTP/1.1\r\n\r\n"))
			buf := make([]byte, 128)
			_, _ = st.Read(buf)
			_ = st.Close()
		}
	})
}
