package security

import (
	"sync"
	"time"
)

// TokenBucketRateLimiter throttles requests using the classic token bucket algorithm.
type TokenBucketRateLimiter struct {
	rate       float64 // tokens per second
	burst      float64 // bucket capacity
	tokens     float64
	lastRefill time.Time
	mu         sync.Mutex
}

// NewTokenBucket creates a single TokenBucketRateLimiter.
func NewTokenBucket(rate, burst float64) *TokenBucketRateLimiter {
	if rate <= 0 {
		rate = 100
	}
	if burst <= 0 {
		burst = rate
	}
	return &TokenBucketRateLimiter{
		rate:       rate,
		burst:      burst,
		tokens:     burst,
		lastRefill: time.Now(),
	}
}

// Allow reports whether a single token is available to proceed.
func (b *TokenBucketRateLimiter) Allow() bool {
	return b.AllowN(time.Now(), 1)
}

// AllowN reports whether n tokens can be consumed at now.
func (b *TokenBucketRateLimiter) AllowN(now time.Time, n float64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	elapsed := now.Sub(b.lastRefill).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * b.rate
		if b.tokens > b.burst {
			b.tokens = b.burst
		}
		b.lastRefill = now
	}

	if b.tokens >= n {
		b.tokens -= n
		return true
	}
	return false
}

// TokenRateLimiterRegistry manages per-token rate limiters dynamically.
type TokenRateLimiterRegistry struct {
	mu           sync.RWMutex
	limiters     map[string]*TokenBucketRateLimiter
	defaultRate  float64
	defaultBurst float64
}

// NewTokenRateLimiterRegistry creates a registry with default rates per second.
func NewTokenRateLimiterRegistry(defaultRate, defaultBurst float64) *TokenRateLimiterRegistry {
	if defaultRate <= 0 {
		defaultRate = 100
	}
	if defaultBurst <= 0 {
		defaultBurst = defaultRate
	}
	return &TokenRateLimiterRegistry{
		limiters:     make(map[string]*TokenBucketRateLimiter),
		defaultRate:  defaultRate,
		defaultBurst: defaultBurst,
	}
}

// Allow checks rate limit for the given token identifier.
func (r *TokenRateLimiterRegistry) Allow(tokenHash string) bool {
	return r.AllowWithCustomRate(tokenHash, r.defaultRate, r.defaultBurst)
}

// AllowWithCustomRate checks rate limit, initializing bucket with custom rate and burst if not present.
func (r *TokenRateLimiterRegistry) AllowWithCustomRate(tokenHash string, rate, burst float64) bool {
	if tokenHash == "" {
		tokenHash = "anonymous"
	}

	r.mu.RLock()
	bucket, ok := r.limiters[tokenHash]
	r.mu.RUnlock()

	if !ok {
		r.mu.Lock()
		bucket, ok = r.limiters[tokenHash]
		if !ok {
			if rate <= 0 {
				rate = r.defaultRate
			}
			if burst <= 0 {
				burst = r.defaultBurst
			}
			bucket = NewTokenBucket(rate, burst)
			r.limiters[tokenHash] = bucket
		}
		r.mu.Unlock()
	}

	return bucket.Allow()
}
