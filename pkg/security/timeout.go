package security

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// TimeoutConfig configures connection idle and request timeouts.
type TimeoutConfig struct {
	ReadHeaderTimeout time.Duration
	IdleTimeout       time.Duration
	StreamTimeout     time.Duration
}

// DefaultTimeoutConfig provides standard timeout values for edge proxying.
func DefaultTimeoutConfig() TimeoutConfig {
	return TimeoutConfig{
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
		StreamTimeout:     30 * time.Second,
	}
}

// TimeoutMiddleware enforces a context deadline on each HTTP request stream.
func TimeoutMiddleware(timeout time.Duration, next http.Handler) http.Handler {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		r = r.WithContext(ctx)

		done := make(chan struct{})
		go func() {
			defer close(done)
			next.ServeHTTP(w, r)
		}()

		select {
		case <-done:
			return
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				http.Error(w, fmt.Sprintf("504 Gateway Timeout: request exceeded %v timeout", timeout), http.StatusGatewayTimeout)
			}
		}
	})
}

// IdleStreamPruner monitors streams and closes inactive connections exceeding idle timeout.
type IdleStreamPruner struct {
	idleTimeout time.Duration
	streams     map[net.Conn]time.Time
	mu          sync.Mutex
	stopCh      chan struct{}
}

// NewIdleStreamPruner creates an IdleStreamPruner.
func NewIdleStreamPruner(idleTimeout time.Duration) *IdleStreamPruner {
	if idleTimeout <= 0 {
		idleTimeout = 60 * time.Second
	}
	return &IdleStreamPruner{
		idleTimeout: idleTimeout,
		streams:     make(map[net.Conn]time.Time),
		stopCh:      make(chan struct{}),
	}
}

// Register registers an active stream connection.
func (p *IdleStreamPruner) Register(conn net.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.streams[conn] = time.Now()
}

// Touch refreshes the active timestamp of a stream.
func (p *IdleStreamPruner) Touch(conn net.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.streams[conn]; ok {
		p.streams[conn] = time.Now()
	}
}

// Unregister removes a stream from tracking.
func (p *IdleStreamPruner) Unregister(conn net.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.streams, conn)
}

// PruneCloses checks and closes connections that have been idle longer than idleTimeout.
func (p *IdleStreamPruner) PruneCloses(now time.Time) int {
	p.mu.Lock()
	defer p.mu.Unlock()

	pruned := 0
	for conn, lastActive := range p.streams {
		if now.Sub(lastActive) > p.idleTimeout {
			_ = conn.Close()
			delete(p.streams, conn)
			pruned++
		}
	}
	return pruned
}
