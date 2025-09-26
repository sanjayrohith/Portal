package proxy

import (
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestStreamBridge_PipeBidirectional(t *testing.T) {
	c1Server, c1Client := net.Pipe()
	defer c1Server.Close()
	defer c1Client.Close()

	c2Server, c2Client := net.Pipe()
	defer c2Server.Close()
	defer c2Client.Close()

	bridge := NewStreamBridge(nil, nil, 1*time.Second)

	go func() {
		_ = bridge.PipeBidirectional(c1Server, c2Server)
	}()

	msg1 := "hello from edge"
	msg2 := "hello from upstream"

	go func() {
		_, _ = c1Client.Write([]byte(msg1))
	}()

	buf := make([]byte, 1024)
	n, err := c2Client.Read(buf)
	if err != nil {
		t.Fatalf("c2Client read failed: %v", err)
	}
	if string(buf[:n]) != msg1 {
		t.Errorf("expected %q, got %q", msg1, string(buf[:n]))
	}

	go func() {
		_, _ = c2Client.Write([]byte(msg2))
	}()

	n, err = c1Client.Read(buf)
	if err != nil {
		t.Fatalf("c1Client read failed: %v", err)
	}
	if string(buf[:n]) != msg2 {
		t.Errorf("expected %q, got %q", msg2, string(buf[:n]))
	}
}

func TestStreamBridge_DialFailureBadGateway(t *testing.T) {
	failDialer := NewLocalDialer("127.0.0.1:1", 100*time.Millisecond)
	bridge := NewStreamBridge(failDialer, nil, 1*time.Second)

	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	go func() {
		_ = bridge.BridgeStream(context.Background(), c1)
	}()

	respBytes, err := io.ReadAll(c2)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	if !strings.Contains(string(respBytes), "502 Bad Gateway") {
		t.Errorf("expected 502 Bad Gateway in response, got %s", string(respBytes))
	}
}
