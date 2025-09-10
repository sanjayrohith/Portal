package mux

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/sanjayrohith/portal/pkg/protocol"
)

type mockFrameSender struct {
	mu           sync.Mutex
	frames       []*protocol.Frame
	closedStream []uint32
}

func (m *mockFrameSender) sendFrame(f *protocol.Frame) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.frames = append(m.frames, f)
	return nil
}

func (m *mockFrameSender) onStreamClosed(id uint32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closedStream = append(m.closedStream, id)
}

func (m *mockFrameSender) localAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}
}

func (m *mockFrameSender) remoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5678}
}

func TestFlowControllerReplenishment(t *testing.T) {
	sender := &mockFrameSender{}
	fc := newFlowController(1, 1000, sender)

	// Threshold is 500
	if err := fc.notifyConsumed(400); err != nil {
		t.Fatalf("notifyConsumed error: %v", err)
	}

	sender.mu.Lock()
	count := len(sender.frames)
	sender.mu.Unlock()
	if count != 0 {
		t.Fatalf("expected 0 frames before threshold, got %d", count)
	}

	// Consume another 150 bytes -> total 550 >= 500
	if err := fc.notifyConsumed(150); err != nil {
		t.Fatalf("notifyConsumed error: %v", err)
	}

	sender.mu.Lock()
	count = len(sender.frames)
	var lastFrame *protocol.Frame
	if count > 0 {
		lastFrame = sender.frames[0]
	}
	sender.mu.Unlock()

	if count != 1 {
		t.Fatalf("expected 1 WINDOW_UPDATE frame, got %d", count)
	}
	credit, err := lastFrame.ParseWindowUpdateCredit()
	if err != nil {
		t.Fatalf("ParseWindowUpdateCredit error: %v", err)
	}
	if credit != 550 {
		t.Fatalf("expected 550 replenished credits, got %d", credit)
	}
}

func TestFlowControllerBlockingAndCredit(t *testing.T) {
	sender := &mockFrameSender{}
	fc := newFlowController(1, 100, sender)
	closeCh := make(chan struct{})

	// Acquire all 100 bytes of credit
	n, err := fc.acquireSendCredit(100, closeCh)
	if err != nil || n != 100 {
		t.Fatalf("expected 100 credits, got %d, err: %v", n, err)
	}

	// Next acquire should block until credit added
	blockedAcquired := make(chan int, 1)
	go func() {
		acquired, _ := fc.acquireSendCredit(50, closeCh)
		blockedAcquired <- acquired
	}()

	select {
	case <-blockedAcquired:
		t.Fatalf("acquireSendCredit should have blocked due to zero window")
	case <-time.After(50 * time.Millisecond):
		// Expected to still be blocked
	}

	// Replenish credit
	fc.addSendCredit(50)

	select {
	case acquired := <-blockedAcquired:
		if acquired != 50 {
			t.Fatalf("expected 50 credits unblocked, got %d", acquired)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("acquireSendCredit timed out waiting for credit")
	}
}
