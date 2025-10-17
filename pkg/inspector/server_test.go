package inspector

import (
	"context"
	"net"
	"testing"
)

func TestNewServerDefaultsToLoopback4040(t *testing.T) {
	server := NewServer(NewRingBuffer(1))
	if got, want := server.Addr(), DefaultInspectorAddr; got != want {
		t.Fatalf("Addr() = %q, want %q", got, want)
	}
	if server.Handler() == nil {
		t.Fatal("Handler() returned nil")
	}
}

func TestNewServerWithAddrRejectsNonLoopback(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:4040", ":4040", "localhost:4040", "[::1]:4040", "127.0.0.1"} {
		if _, err := NewServerWithAddr(addr, nil); err == nil {
			t.Errorf("NewServerWithAddr(%q) succeeded, want rejection", addr)
		}
	}
}

func TestServerListenUsesLoopback(t *testing.T) {
	server, err := NewServerWithAddr("127.0.0.1:0", NewRingBuffer(1))
	if err != nil {
		t.Fatal(err)
	}
	listener, err := server.Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	host, _, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if host != "127.0.0.1" {
		t.Fatalf("listener host = %q, want 127.0.0.1", host)
	}
	if err := server.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() = %v", err)
	}
}
