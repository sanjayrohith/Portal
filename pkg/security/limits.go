package security

import (
	"errors"
	"fmt"
	"sync"

	portalErr "github.com/sanjayrohith/portal/pkg/errors"
)

var (
	// ErrMaxTunnelsExceeded indicates the client token has reached its active tunnel quota.
	ErrMaxTunnelsExceeded = errors.New("security: max concurrent tunnels quota exceeded")
	// ErrMaxStreamsExceeded indicates the tunnel session has reached its concurrent stream quota.
	ErrMaxStreamsExceeded = errors.New("security: max concurrent streams limit exceeded")
)

// QuotaManager tracks and enforces active concurrent tunnels and stream limits per token.
type QuotaManager struct {
	mu            sync.RWMutex
	activeTunnels map[string]int // tokenHash -> active tunnels count
	activeStreams map[string]int // tokenHash -> active streams count
	defaultMax    int
}

// NewQuotaManager creates a new QuotaManager with a fallback max quota.
func NewQuotaManager(defaultMax int) *QuotaManager {
	if defaultMax <= 0 {
		defaultMax = 5
	}
	return &QuotaManager{
		activeTunnels: make(map[string]int),
		activeStreams: make(map[string]int),
		defaultMax:    defaultMax,
	}
}

// AcquireTunnel attempts to increment the active tunnel count for a token respecting maxLimit.
func (q *QuotaManager) AcquireTunnel(tokenHash string, maxLimit int) error {
	if maxLimit <= 0 {
		maxLimit = q.defaultMax
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	current := q.activeTunnels[tokenHash]
	if current >= maxLimit {
		return fmt.Errorf("%w: active %d reaches max %d", portalErr.ErrQuotaExceeded, current, maxLimit)
	}

	q.activeTunnels[tokenHash] = current + 1
	return nil
}

// ReleaseTunnel decrements the active tunnel count for a token.
func (q *QuotaManager) ReleaseTunnel(tokenHash string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	current := q.activeTunnels[tokenHash]
	if current <= 1 {
		delete(q.activeTunnels, tokenHash)
	} else {
		q.activeTunnels[tokenHash] = current - 1
	}
}

// ActiveTunnels returns the current count of active tunnels for a token.
func (q *QuotaManager) ActiveTunnels(tokenHash string) int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.activeTunnels[tokenHash]
}

// AcquireStream attempts to increment concurrent stream count for a token respecting maxLimit.
func (q *QuotaManager) AcquireStream(tokenHash string, maxLimit int) error {
	if maxLimit <= 0 {
		maxLimit = 100 // default stream limit per tunnel token
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	current := q.activeStreams[tokenHash]
	if current >= maxLimit {
		return ErrMaxStreamsExceeded
	}

	q.activeStreams[tokenHash] = current + 1
	return nil
}

// ReleaseStream decrements active stream count for a token.
func (q *QuotaManager) ReleaseStream(tokenHash string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	current := q.activeStreams[tokenHash]
	if current <= 1 {
		delete(q.activeStreams, tokenHash)
	} else {
		q.activeStreams[tokenHash] = current - 1
	}
}

// ActiveStreams returns the active streams for a token.
func (q *QuotaManager) ActiveStreams(tokenHash string) int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.activeStreams[tokenHash]
}
