package proxy

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestLocalDialer(t *testing.T) {
	// Setup dummy TCP server
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	dialer := NewLocalDialer(ln.Addr().String(), 1*time.Second)
	if dialer.Target() != ln.Addr().String() {
		t.Errorf("expected target %s, got %s", ln.Addr().String(), dialer.Target())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := dialer.Dial(ctx)
	if err != nil {
		t.Fatalf("dialer.Dial failed: %v", err)
	}
	_ = conn.Close()

	// Test dial failure on closed port
	failDialer := NewLocalDialer("127.0.0.1:1", 200*time.Millisecond)
	_, err = failDialer.Dial(ctx)
	if err == nil {
		t.Errorf("expected dial failure on closed port")
	}
}

func TestConnectionPool(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			// Keep alive until client closes
			buf := make([]byte, 1)
			_, _ = conn.Read(buf)
			_ = conn.Close()
		}
	}()

	dialer := NewLocalDialer(ln.Addr().String(), 1*time.Second)
	pool := NewConnectionPool(dialer, 2)
	defer pool.Close()

	ctx := context.Background()
	c1, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire c1 failed: %v", err)
	}

	pool.Release(c1)

	c2, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire c2 failed: %v", err)
	}
	if c2 != c1 {
		t.Errorf("expected pooled connection to be reused")
	}
	pool.Release(c2)
}
