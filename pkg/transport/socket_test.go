package transport

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestTuneConnAppliesTCPOptions(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no loopback TCP: %v", err)
	}
	defer ln.Close()

	serverCh := make(chan net.Conn, 1)
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			serverCh <- conn
		}
	}()

	conn, err := DialTuned(context.Background(), "tcp", ln.Addr().String(), 5*time.Second, DefaultSocketOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	server := <-serverCh
	defer server.Close()
	if err := TuneConn(server, DefaultSocketOptions()); err != nil {
		t.Fatal(err)
	}

	tcp, ok := conn.(*net.TCPConn)
	if !ok {
		t.Fatal("expected *net.TCPConn")
	}
	_ = tcp
}

func TestTuneConnSkipsNonTCP(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	if err := TuneConn(c1, DefaultSocketOptions()); err != nil {
		t.Fatalf("non-TCP conn should be skipped, got %v", err)
	}
}

func TestTunedListenerAccepts(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no loopback TCP: %v", err)
	}
	defer ln.Close()

	tuned := TuneListener(ln, DefaultSocketOptions())
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := net.Dial("tcp", tuned.Addr().String())
		if err == nil {
			conn.Close()
		}
	}()
	conn, err := tuned.Accept()
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	<-done
}
