package resilience

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/sanjayrohith/portal/pkg/mux"
)

func TestInFlightTracker_Drain(t *testing.T) {
	tracker := NewInFlightTracker()

	var wg sync.WaitGroup
	const workers = 5

	for i := 0; i < workers; i++ {
		wg.Add(1)
		done := tracker.Track()
		go func() {
			defer wg.Done()
			defer done()
			time.Sleep(30 * time.Millisecond)
		}()
	}

	if tracker.InFlightCount() != int64(workers) {
		t.Errorf("expected %d in flight, got %d", workers, tracker.InFlightCount())
	}

	ctx := context.Background()
	err := tracker.Drain(ctx, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("Drain failed: %v", err)
	}

	if tracker.InFlightCount() != 0 {
		t.Errorf("expected 0 in flight, got %d", tracker.InFlightCount())
	}

	wg.Wait()
}

func TestSessionReconciler(t *testing.T) {
	tracker := NewInFlightTracker()
	reconciler := NewSessionReconciler(tracker)

	c1, s1 := net.Pipe()
	oldSession := mux.NewSession(c1, false)
	defer s1.Close()

	c2, s2 := net.Pipe()
	newSession := mux.NewSession(c2, false)
	defer c2.Close()
	defer s2.Close()

	// Track a short-lived operation
	done := tracker.Track()
	go func() {
		time.Sleep(20 * time.Millisecond)
		done()
	}()

	err := reconciler.Reconcile(context.Background(), oldSession, newSession, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	if !oldSession.IsClosed() {
		t.Errorf("expected oldSession to be closed after reconciliation")
	}

	if newSession.IsClosed() {
		t.Errorf("expected newSession to remain open")
	}
}
