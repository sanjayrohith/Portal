package resilience

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sanjayrohith/portal/pkg/mux"
)

// HealthStatus represents the current health state of the tunnel connection.
type HealthStatus int

const (
	StatusHealthy HealthStatus = iota
	StatusDegraded
	StatusDead
)

func (s HealthStatus) String() string {
	switch s {
	case StatusHealthy:
		return "HEALTHY"
	case StatusDegraded:
		return "DEGRADED"
	case StatusDead:
		return "DEAD"
	default:
		return fmt.Sprintf("HealthStatus(%d)", s)
	}
}

// HealthWatchdog monitors connection liveness via session heartbeats, ping responses, and transport drops.
type HealthWatchdog struct {
	session            *mux.Session
	interval           time.Duration
	pingTimeout        time.Duration
	maxMissedPings     int
	status             atomic.Int32
	missedPingsCount   atomic.Int32
	disruptionListener chan struct{}
	lastRTT            atomic.Int64 // nanoseconds
	stopCh             chan struct{}
	wg                 sync.WaitGroup
	mu                 sync.Mutex
	running            bool
}

// NewHealthWatchdog creates a new HealthWatchdog for a multiplexer session.
func NewHealthWatchdog(session *mux.Session, interval, pingTimeout time.Duration, maxMissed int) *HealthWatchdog {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if pingTimeout <= 0 {
		pingTimeout = 2 * time.Second
	}
	if maxMissed <= 0 {
		maxMissed = 3
	}

	w := &HealthWatchdog{
		session:            session,
		interval:           interval,
		pingTimeout:        pingTimeout,
		maxMissedPings:     maxMissed,
		disruptionListener: make(chan struct{}, 1),
		stopCh:             make(chan struct{}),
	}
	w.status.Store(int32(StatusHealthy))
	return w
}

// Start begins periodic connection health monitoring in the background.
func (w *HealthWatchdog) Start(ctx context.Context) {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.mu.Unlock()

	w.wg.Add(1)
	go w.monitorLoop(ctx)
}

func (w *HealthWatchdog) monitorLoop(ctx context.Context) {
	defer w.wg.Done()
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.notifyDisruption()
			return
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.probe()
		}
	}
}

func (w *HealthWatchdog) probe() {
	if w.session == nil || w.session.IsClosed() {
		w.status.Store(int32(StatusDead))
		w.notifyDisruption()
		return
	}

	rtt, err := w.session.Ping(w.pingTimeout)
	if err != nil {
		missed := w.missedPingsCount.Add(1)
		if int(missed) >= w.maxMissedPings {
			w.status.Store(int32(StatusDead))
			w.notifyDisruption()
		} else {
			w.status.Store(int32(StatusDegraded))
		}
		return
	}

	w.missedPingsCount.Store(0)
	w.lastRTT.Store(rtt.Nanoseconds())
	w.status.Store(int32(StatusHealthy))
}

func (w *HealthWatchdog) notifyDisruption() {
	select {
	case w.disruptionListener <- struct{}{}:
	default:
	}
}

// DisruptionChan returns a channel that signals when network connection drops or misses threshold pings.
func (w *HealthWatchdog) DisruptionChan() <-chan struct{} {
	return w.disruptionListener
}

// Status returns the current HealthStatus.
func (w *HealthWatchdog) Status() HealthStatus {
	return HealthStatus(w.status.Load())
}

// LastRTT returns the most recent heartbeat RTT.
func (w *HealthWatchdog) LastRTT() time.Duration {
	return time.Duration(w.lastRTT.Load())
}

// Stop terminates the monitoring loop.
func (w *HealthWatchdog) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.running = false
	close(w.stopCh)
	w.mu.Unlock()

	w.wg.Wait()
}
