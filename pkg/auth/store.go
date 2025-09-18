package auth

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// TokenStore defines the storage and validation contract for authentication tokens.
type TokenStore interface {
	RegisterToken(token *Token) error
	RegisterRawToken(rawToken string, owner string, maxTunnels, rateLimit int) (*Token, error)
	ValidateToken(rawToken string) (*Token, error)
	RevokeToken(rawToken string) error
	ListTokens() []*Token
	LoadPreProvisionedTokens(rawTokens []string) error
}

// MemoryTokenStore provides a thread-safe in-memory implementation of TokenStore.
type MemoryTokenStore struct {
	mu     sync.RWMutex
	tokens map[string]*Token // Keyed by SHA-256 hash hex string
}

// NewMemoryTokenStore constructs a new MemoryTokenStore.
func NewMemoryTokenStore() *MemoryTokenStore {
	return &MemoryTokenStore{
		tokens: make(map[string]*Token),
	}
}

// RegisterToken registers a Token structure into the store.
func (s *MemoryTokenStore) RegisterToken(token *Token) error {
	if token == nil {
		return fmt.Errorf("cannot register nil token")
	}
	if token.Hash == "" {
		return fmt.Errorf("token hash cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.tokens[token.Hash] = token
	return nil
}

// RegisterRawToken hashes and registers a raw token string with ownership and quotas.
func (s *MemoryTokenStore) RegisterRawToken(rawToken string, owner string, maxTunnels, rateLimit int) (*Token, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return nil, ErrMissingToken
	}

	hash := HashToken(rawToken)
	token := &Token{
		ID:         hash[:12],
		Hash:       hash,
		Owner:      owner,
		MaxTunnels: maxTunnels,
		RateLimit:  rateLimit,
		Revoked:    false,
		CreatedAt:  time.Now(),
	}

	s.mu.Lock()
	s.tokens[hash] = token
	s.mu.Unlock()

	return token, nil
}

// ValidateToken verifies rawToken against stored tokens and checks its validity.
func (s *MemoryTokenStore) ValidateToken(rawToken string) (*Token, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return nil, ErrMissingToken
	}

	hash := HashToken(rawToken)

	s.mu.RLock()
	token, exists := s.tokens[hash]
	s.mu.RUnlock()

	if !exists {
		return nil, ErrInvalidToken
	}

	// Constant-time comparison check
	if !VerifyToken(rawToken, token.Hash) {
		return nil, ErrInvalidToken
	}

	if token.Revoked {
		return nil, ErrTokenRevoked
	}

	if token.IsExpired() {
		return nil, ErrTokenExpired
	}

	return token, nil
}

// RevokeToken flags a token as revoked, immediately invalidating any new session attempts.
func (s *MemoryTokenStore) RevokeToken(rawToken string) error {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return ErrMissingToken
	}

	hash := HashToken(rawToken)

	s.mu.Lock()
	defer s.mu.Unlock()

	token, exists := s.tokens[hash]
	if !exists {
		return ErrInvalidToken
	}

	token.Revoked = true
	return nil
}

// ListTokens returns a snapshot list of all registered tokens.
func (s *MemoryTokenStore) ListTokens() []*Token {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*Token, 0, len(s.tokens))
	for _, t := range s.tokens {
		// Return copy to prevent external mutation
		cp := *t
		list = append(list, &cp)
	}
	return list
}

// LoadPreProvisionedTokens loads raw tokens from static configuration slices.
func (s *MemoryTokenStore) LoadPreProvisionedTokens(rawTokens []string) error {
	for i, raw := range rawTokens {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		owner := fmt.Sprintf("pre-provisioned-user-%d", i+1)
		if _, err := s.RegisterRawToken(trimmed, owner, 5, 100); err != nil {
			return fmt.Errorf("failed to register pre-provisioned token %d: %w", i, err)
		}
	}
	return nil
}
