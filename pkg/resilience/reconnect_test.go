package resilience

import (
	"context"
	"testing"
	"time"
)

func TestReconnectSupervisor_ExponentialGrowthAndJitter(t *testing.T) {
	cfg := BackoffConfig{
		InitialInterval: 100 * time.Millisecond,
		MaxInterval:     1 * time.Second,
		Multiplier:      2.0,
		MaxJitterRatio:  0.1,
		MaxRetries:      4,
	}

	sup := NewReconnectSupervisor(cfg)

	// Attempt 1: ~100ms
	d1, ok := sup.NextDelay()
	if !ok || d1 < 80*time.Millisecond || d1 > 120*time.Millisecond {
		t.Errorf("unexpected d1: %v", d1)
	}

	// Attempt 2: ~200ms
	d2, ok := sup.NextDelay()
	if !ok || d2 < 160*time.Millisecond || d2 > 240*time.Millisecond {
		t.Errorf("unexpected d2: %v", d2)
	}

	// Attempt 3: ~400ms
	d3, ok := sup.NextDelay()
	if !ok || d3 < 320*time.Millisecond || d3 > 480*time.Millisecond {
		t.Errorf("unexpected d3: %v", d3)
	}

	// Attempt 4: ~800ms
	d4, ok := sup.NextDelay()
	if !ok || d4 < 640*time.Millisecond || d4 > 960*time.Millisecond {
		t.Errorf("unexpected d4: %v", d4)
	}

	// Attempt 5 should exceed MaxRetries=4
	_, ok = sup.NextDelay()
	if ok {
		t.Errorf("expected max retries exhausted on 5th call")
	}

	// Reset clears state
	sup.Reset()
	if sup.Attempts() != 0 {
		t.Errorf("expected 0 attempts after reset, got %d", sup.Attempts())
	}
}

func TestReconnectSupervisor_ContextCancellation(t *testing.T) {
	cfg := BackoffConfig{
		InitialInterval: 500 * time.Millisecond,
		MaxInterval:     1 * time.Second,
		Multiplier:      1.5,
		MaxJitterRatio:  0.0,
		MaxRetries:      10,
	}
	sup := NewReconnectSupervisor(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	res := sup.Sleep(ctx)
	if res {
		t.Errorf("expected false when context is cancelled early")
	}
	if time.Since(start) > 200*time.Millisecond {
		t.Errorf("Sleep did not terminate promptly on context cancellation")
	}
}
