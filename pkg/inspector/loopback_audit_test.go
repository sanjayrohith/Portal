package inspector

import (
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestServeRejectsNonLoopbackListener(t *testing.T) {
	server := NewServer(NewRingBuffer(10))

	external, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Skipf("cannot bind wildcard listener: %v", err)
	}
	defer external.Close()

	errCh := make(chan error, 1)
	go func() { errCh <- server.Serve(external) }()
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("Serve on 0.0.0.0 listener succeeded, want loopback rejection")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not reject the non-loopback listener promptly")
	}
}

func TestServeAcceptsLoopbackListener(t *testing.T) {
	server := NewServer(NewRingBuffer(10))
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() { _ = server.Serve(listener) }()

	resp, err := http.Get("http://" + listener.Addr().String() + "/api/requests")
	if err != nil {
		t.Fatalf("loopback inspector request failed: %v", err)
	}
	defer resp.Body.Close()
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("inspector status = %d, want 200", resp.StatusCode)
	}
}

func TestListenValidatesAddress(t *testing.T) {
	server := &Server{addr: "0.0.0.0:4040", buffer: NewRingBuffer(1)}
	if _, err := server.Listen(); err == nil {
		t.Fatal("Listen with wildcard address succeeded, want rejection")
	}
}
