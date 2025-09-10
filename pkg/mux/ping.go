package mux

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	portalErr "github.com/sanjayrohith/portal/pkg/errors"
	"github.com/sanjayrohith/portal/pkg/protocol"
)

// pingWaiter tracks a single in-flight ping request.
type pingWaiter struct {
	sentAt time.Time
	ch     chan time.Time
}

// pingManager manages PING/PONG heartbeats, RTT calculations, and dead connection detection.
type pingManager struct {
	session *Session

	mu      sync.Mutex
	waiters map[uint32]*pingWaiter

	lastRTT atomic.Int64 // nanoseconds
	rng     *rand.Rand
}

func newPingManager(s *Session) *pingManager {
	pm := &pingManager{
		session: s,
		waiters: make(map[uint32]*pingWaiter),
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	// Register with session to receive PONG frames
	s.pongMu.Lock()
	s.pongHandler = pm.handlePong
	s.pongMu.Unlock()

	return pm
}

// Ping sends a PING frame with a unique nonce and waits for the matching PONG frame.
func (pm *pingManager) Ping(timeout time.Duration) (time.Duration, error) {
	if pm.session.IsClosed() {
		return 0, portalErr.ErrConnectionClosed
	}

	pm.mu.Lock()
	nonce := pm.rng.Uint32()
	for _, exists := pm.waiters[nonce]; exists; _, exists = pm.waiters[nonce] {
		nonce = pm.rng.Uint32()
	}

	ch := make(chan time.Time, 1)
	waiter := &pingWaiter{
		sentAt: time.Now(),
		ch:     ch,
	}
	pm.waiters[nonce] = waiter
	pm.mu.Unlock()

	pingFrame := protocol.NewPingFrame(nonce)
	if err := pm.session.sendFrame(pingFrame); err != nil {
		pm.mu.Lock()
		delete(pm.waiters, nonce)
		pm.mu.Unlock()
		return 0, err
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-pm.session.closeCh:
		pm.mu.Lock()
		delete(pm.waiters, nonce)
		pm.mu.Unlock()
		return 0, portalErr.ErrConnectionClosed

	case <-timer.C:
		pm.mu.Lock()
		delete(pm.waiters, nonce)
		pm.mu.Unlock()
		return 0, fmt.Errorf("ping timed out after %v", timeout)

	case receivedAt := <-ch:
		rtt := receivedAt.Sub(waiter.sentAt)
		pm.lastRTT.Store(rtt.Nanoseconds())
		return rtt, nil
	}
}

// handlePong is invoked by the session receiver loop when a PONG frame arrives.
func (pm *pingManager) handlePong(nonce uint32) {
	pm.mu.Lock()
	waiter, ok := pm.waiters[nonce]
	if ok {
		delete(pm.waiters, nonce)
	}
	pm.mu.Unlock()

	if ok {
		select {
		case waiter.ch <- time.Now():
		default:
		}
	}
}

// LastRTT returns the most recently measured round-trip time.
func (pm *pingManager) LastRTT() time.Duration {
	return time.Duration(pm.lastRTT.Load())
}

// StartKeepalive spawns a background worker that periodically sends PINGs and detects dead connections.
func (pm *pingManager) StartKeepalive(interval time.Duration, timeout time.Duration, maxConsecutiveFailures int) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		consecutiveFailures := 0

		for {
			select {
			case <-pm.session.closeCh:
				return
			case <-ticker.C:
				_, err := pm.Ping(timeout)
				if err != nil {
					consecutiveFailures++
					if consecutiveFailures >= maxConsecutiveFailures {
						_ = pm.session.Close()
						return
					}
				} else {
					consecutiveFailures = 0
				}
			}
		}
	}()
}
