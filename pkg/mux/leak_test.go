package mux

import (
	"io"
	"net"
	"runtime"
	"testing"
	"time"
)

// waitForGoroutines polls until the goroutine count falls back to baseline
// (within slack) or the timeout expires.
func waitForGoroutines(t *testing.T, baseline int, slack int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= baseline+slack {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("goroutine leak: have %d, baseline %d (slack %d)",
		runtime.NumGoroutine(), baseline, slack)
}

func TestStreamOpenCloseCyclingNoLeak(t *testing.T) {
	baseline := runtime.NumGoroutine()

	c1, c2 := net.Pipe()
	serverSession := NewSession(c1, true)
	clientSession := NewSession(c2, false)

	// Server side: accept and immediately close streams.
	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		for {
			stream, err := serverSession.AcceptStream()
			if err != nil {
				return
			}
			_ = stream.Close()
		}
	}()

	const cycles = 200
	for i := 0; i < cycles; i++ {
		stream, err := clientSession.OpenStream()
		if err != nil {
			t.Fatalf("cycle %d: OpenStream: %v", i, err)
		}
		if err := stream.Close(); err != nil {
			t.Fatalf("cycle %d: Close: %v", i, err)
		}
	}

	if n := clientSession.ActiveStreamsCount(); n != 0 {
		t.Fatalf("client session retains %d streams after cycling", n)
	}

	_ = clientSession.Close()
	_ = serverSession.Close()
	<-acceptDone

	waitForGoroutines(t, baseline, 5, 5*time.Second)
}

func TestClosedStreamReleasesReadBuffer(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	serverSession := NewSession(c1, true)
	clientSession := NewSession(c2, false)
	defer serverSession.Close()
	defer clientSession.Close()

	accepted := make(chan *Stream, 1)
	go func() {
		stream, err := serverSession.AcceptStream()
		if err == nil {
			accepted <- stream
		}
	}()

	client, err := clientSession.OpenStream()
	if err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, 4096)
	for i := range payload {
		payload[i] = byte(i)
	}
	if _, err := client.Write(payload); err != nil {
		t.Fatal(err)
	}

	var server *Stream
	select {
	case server = <-accepted:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for accepted stream")
	}

	buf := make([]byte, len(payload))
	if _, err := io.ReadFull(server, buf); err != nil {
		t.Fatal(err)
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	server.readMu.Lock()
	remaining := server.readBuf.Len()
	server.readMu.Unlock()
	if remaining != 0 {
		t.Fatalf("closed stream retains %d buffered bytes", remaining)
	}
	_ = client.Close()
}
