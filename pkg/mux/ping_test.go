package mux

import (
	"net"
	"testing"
	"time"
)

func TestSessionPingPong(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	clientSession := NewSession(clientConn, false)
	serverSession := NewSession(serverConn, true)
	defer clientSession.Close()
	defer serverSession.Close()

	rtt, err := clientSession.Ping(1 * time.Second)
	if err != nil {
		t.Fatalf("Ping failed: %v", err)
	}

	if rtt <= 0 {
		t.Fatalf("expected positive RTT, got %v", rtt)
	}

	lastRTT := clientSession.LastRTT()
	if lastRTT != rtt {
		t.Errorf("expected LastRTT to be %v, got %v", rtt, lastRTT)
	}
}

func TestSessionPingTimeout(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	clientSession := NewSession(clientConn, false)
	defer clientSession.Close()

	// Do not start serverSession, serverConn just discards or blocks
	go func() {
		buf := make([]byte, 1024)
		for {
			if _, err := serverConn.Read(buf); err != nil {
				return
			}
		}
	}()

	_, err := clientSession.Ping(50 * time.Millisecond)
	if err == nil {
		t.Fatalf("expected ping timeout error, got nil")
	}
}
