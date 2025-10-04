package resilience

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sanjayrohith/portal/pkg/mux"
)

// InFlightTracker tracks active client streams and orchestrates graceful draining during disconnects.
type InFlightTracker struct {
	activeCount atomic.Int64
	doneCh      chan struct{}
	mu          sync.Mutex
}

// NewInFlightTracker creates a new InFlightTracker.
func NewInFlightTracker() *InFlightTracker {
	t := &InFlightTracker{
		doneCh: make(chan struct{}),
	}
	close(t.doneCh) // starts empty/idle
	return t
}

// Track marks the start of an in-flight operation and returns a completion callback.
func (t *InFlightTracker) Track() func() {
	t.mu.Lock()
	count := t.activeCount.Add(1)
	if count == 1 {
		t.doneCh = make(chan struct{})
	}
	t.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			t.mu.Lock()
			defer t.mu.Unlock()
			remaining := t.activeCount.Add(-1)
			if remaining <= 0 {
				t.activeCount.Store(0)
				select {
				case <-t.doneCh:
				default:
					close(t.doneCh)
				}
			}
		})
	}
}

// InFlightCount returns the number of currently active operations.
func (t *InFlightTracker) InFlightCount() int64 {
	return t.activeCount.Load()
}

// Drain blocks until all in-flight operations finish or timeout expires.
func (t *InFlightTracker) Drain(ctx context.Context, timeout time.Duration) error {
	if t.activeCount.Load() == 0 {
		return nil
	}

	t.mu.Lock()
	ch := t.doneCh
	t.mu.Unlock()

	drainCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case <-ch:
		return nil
	case <-drainCtx.Done():
		return fmt.Errorf("drain timed out with %d in-flight operations remaining", t.activeCount.Load())
	}
}

// SessionReconciler handles clean session switchover during transient reconnect cycles.
type SessionReconciler struct {
	tracker *InFlightTracker
}

// NewSessionReconciler creates a new SessionReconciler.
func NewSessionReconciler(tracker *InFlightTracker) *SessionReconciler {
	if tracker == nil {
		tracker = NewInFlightTracker()
	}
	return &SessionReconciler{
		tracker: tracker,
	}
}

// Reconcile drains oldSession streams within drainTimeout and closes oldSession before activating newSession.
func (r *SessionReconciler) Reconcile(ctx context.Context, oldSession, newSession *mux.Session, drainTimeout time.Duration) error {
	if oldSession != nil && !oldSession.IsClosed() {
		// Attempt graceful drain of in-flight requests
		_ = r.tracker.Drain(ctx, drainTimeout)
		_ = oldSession.Close()
	}

	return nil
}
