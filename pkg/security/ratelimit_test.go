package security

import (
	"testing"
	"time"
)

func TestTokenBucketRateLimiter(t *testing.T) {
	// 10 tokens/sec, burst 2
	bucket := NewTokenBucket(10, 2)

	now := time.Now()
	// Should allow 2 burst tokens
	if !bucket.AllowN(now, 1) {
		t.Errorf("expected 1st token allowed")
	}
	if !bucket.AllowN(now, 1) {
		t.Errorf("expected 2nd token allowed")
	}
	// 3rd immediately should be rejected
	if bucket.AllowN(now, 1) {
		t.Errorf("expected 3rd token denied due to burst limit")
	}

	// Advance 100ms -> 1 token replenished (10 * 0.1 = 1)
	now = now.Add(100 * time.Millisecond)
	if !bucket.AllowN(now, 1) {
		t.Errorf("expected replenished token allowed after 100ms")
	}
}

func TestTokenRateLimiterRegistry(t *testing.T) {
	reg := NewTokenRateLimiterRegistry(5, 5)

	// Token A
	for i := 0; i < 5; i++ {
		if !reg.Allow("token-A") {
			t.Errorf("token-A: expected attempt %d allowed", i)
		}
	}
	if reg.Allow("token-A") {
		t.Errorf("token-A: expected 6th attempt rejected")
	}

	// Token B isolated from Token A
	if !reg.Allow("token-B") {
		t.Errorf("token-B: expected attempt allowed independently")
	}
}
