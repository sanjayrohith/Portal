package resilience

import (
	"context"
	"crypto/rand"
	"math"
	"math/big"
	"time"
)

// BackoffConfig configures the exponential backoff with jitter algorithm.
type BackoffConfig struct {
	InitialInterval time.Duration
	MaxInterval     time.Duration
	Multiplier      float64
	MaxJitterRatio  float64
	MaxRetries      int // 0 means unlimited
}

// DefaultBackoffConfig provides sensible default parameters for tunnel reconnection.
func DefaultBackoffConfig() BackoffConfig {
	return BackoffConfig{
		InitialInterval: 500 * time.Millisecond,
		MaxInterval:     30 * time.Second,
		Multiplier:      1.5,
		MaxJitterRatio:  0.2,
		MaxRetries:      0,
	}
}

// ReconnectSupervisor calculates delays and supervises reconnect retry loops with exponential backoff and jitter.
type ReconnectSupervisor struct {
	cfg      BackoffConfig
	attempts int
}

// NewReconnectSupervisor creates a new ReconnectSupervisor with the given configuration.
func NewReconnectSupervisor(cfg BackoffConfig) *ReconnectSupervisor {
	if cfg.InitialInterval <= 0 {
		cfg.InitialInterval = 500 * time.Millisecond
	}
	if cfg.MaxInterval <= 0 {
		cfg.MaxInterval = 30 * time.Second
	}
	if cfg.Multiplier <= 1.0 {
		cfg.Multiplier = 1.5
	}
	if cfg.MaxJitterRatio < 0 || cfg.MaxJitterRatio > 1.0 {
		cfg.MaxJitterRatio = 0.2
	}
	return &ReconnectSupervisor{
		cfg: cfg,
	}
}

// NextDelay calculates the duration to wait before the next reconnection attempt.
// Returns delay and whether retries have been exhausted.
func (s *ReconnectSupervisor) NextDelay() (time.Duration, bool) {
	if s.cfg.MaxRetries > 0 && s.attempts >= s.cfg.MaxRetries {
		return 0, false
	}

	// Exponential backoff calculation: initial * multiplier^attempts
	delaySec := float64(s.cfg.InitialInterval) / float64(time.Second) * math.Pow(s.cfg.Multiplier, float64(s.attempts))
	delay := time.Duration(delaySec * float64(time.Second))

	if delay > s.cfg.MaxInterval {
		delay = s.cfg.MaxInterval
	}

	// Add full randomized jitter: delay +/- (delay * jitterRatio)
	if s.cfg.MaxJitterRatio > 0 {
		maxJitterNs := int64(float64(delay.Nanoseconds()) * s.cfg.MaxJitterRatio)
		if maxJitterNs > 0 {
			n, err := rand.Int(rand.Reader, big.NewInt(maxJitterNs*2))
			if err == nil {
				jitterOffset := n.Int64() - maxJitterNs
				delay = time.Duration(int64(delay) + jitterOffset)
			}
		}
	}

	if delay < s.cfg.InitialInterval/2 {
		delay = s.cfg.InitialInterval / 2
	}

	s.attempts++
	return delay, true
}

// Attempts returns the count of reconnect attempts executed so far.
func (s *ReconnectSupervisor) Attempts() int {
	return s.attempts
}

// Reset clears the backoff attempt count following a successful connection.
func (s *ReconnectSupervisor) Reset() {
	s.attempts = 0
}

// Sleep waits for the next delay or exits early if ctx is canceled.
func (s *ReconnectSupervisor) Sleep(ctx context.Context) bool {
	delay, ok := s.NextDelay()
	if !ok {
		return false
	}

	select {
	case <-ctx.Done():
		return false
	case <-time.After(delay):
		return true
	}
}
