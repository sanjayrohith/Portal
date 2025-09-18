package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrMissingToken indicates that no auth token was provided.
	ErrMissingToken = errors.New("auth: authentication token required")

	// ErrInvalidToken indicates that the token hash did not match any registered token.
	ErrInvalidToken = errors.New("auth: invalid authentication token")

	// ErrTokenRevoked indicates that the token has been explicitly revoked.
	ErrTokenRevoked = errors.New("auth: authentication token revoked")

	// ErrTokenExpired indicates that the token's lifetime has expired.
	ErrTokenExpired = errors.New("auth: authentication token expired")
)

// Token represents an authentication token with ownership and quota metadata.
type Token struct {
	ID           string    `json:"id"`
	Hash         string    `json:"hash"`
	Owner        string    `json:"owner"`
	MaxTunnels   int       `json:"max_tunnels"`
	RateLimit    int       `json:"rate_limit"`
	Revoked      bool      `json:"revoked"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
}

// HashToken computes a standard SHA-256 hex digest of the raw token string.
func HashToken(rawToken string) string {
	rawToken = strings.TrimSpace(rawToken)
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}

// VerifyToken compares a raw token against an expected SHA-256 hex hash in constant time.
func VerifyToken(rawToken, expectedHash string) bool {
	computed := HashToken(rawToken)
	return subtle.ConstantTimeCompare([]byte(computed), []byte(expectedHash)) == 1
}

// GenerateSecureToken generates a cryptographically secure random authentication token.
// The resulting format is "pt_<32 random hex bytes>".
func GenerateSecureToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate secure random bytes: %w", err)
	}
	return "pt_" + hex.EncodeToString(bytes), nil
}

// IsExpired reports whether the token's expiration date has passed.
func (t *Token) IsExpired() bool {
	if t.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(t.ExpiresAt)
}

// IsValid reports whether the token is active, unrevoked, and unexpired.
func (t *Token) IsValid() bool {
	if t.Revoked {
		return false
	}
	return !t.IsExpired()
}
