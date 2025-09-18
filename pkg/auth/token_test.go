package auth

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHashAndVerifyToken(t *testing.T) {
	raw := "pt_secret_test_token_12345"
	hash := HashToken(raw)

	if hash == "" {
		t.Fatalf("HashToken returned empty string")
	}

	if !VerifyToken(raw, hash) {
		t.Errorf("VerifyToken failed for matching token and hash")
	}

	if VerifyToken("pt_wrong_token", hash) {
		t.Errorf("VerifyToken succeeded for mismatched token")
	}
}

func TestGenerateSecureToken(t *testing.T) {
	seen := make(map[string]bool)
	const count = 100

	for i := 0; i < count; i++ {
		tok, err := GenerateSecureToken()
		if err != nil {
			t.Fatalf("GenerateSecureToken error: %v", err)
		}

		if !strings.HasPrefix(tok, "pt_") {
			t.Errorf("expected token to start with 'pt_', got %s", tok)
		}

		// "pt_" + 64 hex characters (32 bytes) = 67 characters
		if len(tok) != 67 {
			t.Errorf("expected token length 67, got %d", len(tok))
		}

		if seen[tok] {
			t.Fatalf("collision detected in generated token: %s", tok)
		}
		seen[tok] = true
	}
}

func TestTokenValidity(t *testing.T) {
	now := time.Now()

	validToken := &Token{
		ID:        "tok-1",
		Revoked:   false,
		ExpiresAt: now.Add(1 * time.Hour),
	}
	if !validToken.IsValid() {
		t.Errorf("expected valid token to report IsValid() == true")
	}

	revokedToken := &Token{
		ID:        "tok-2",
		Revoked:   true,
		ExpiresAt: now.Add(1 * time.Hour),
	}
	if revokedToken.IsValid() {
		t.Errorf("expected revoked token to report IsValid() == false")
	}

	expiredToken := &Token{
		ID:        "tok-3",
		Revoked:   false,
		ExpiresAt: now.Add(-1 * time.Hour),
	}
	if expiredToken.IsValid() {
		t.Errorf("expected expired token to report IsValid() == false")
	}
	if !expiredToken.IsExpired() {
		t.Errorf("expected expired token to report IsExpired() == true")
	}
}

func TestMemoryTokenStoreTableDriven(t *testing.T) {
	store := NewMemoryTokenStore()

	// Provision valid token
	validRaw := "pt_valid_token_001"
	_, err := store.RegisterRawToken(validRaw, "alice", 5, 100)
	if err != nil {
		t.Fatalf("failed registering valid token: %v", err)
	}

	// Provision revoked token
	revokedRaw := "pt_revoked_token_002"
	_, err = store.RegisterRawToken(revokedRaw, "bob", 3, 50)
	if err != nil {
		t.Fatalf("failed registering revoked token: %v", err)
	}
	if err := store.RevokeToken(revokedRaw); err != nil {
		t.Fatalf("failed to revoke token: %v", err)
	}

	// Provision expired token
	expiredRaw := "pt_expired_token_003"
	expHash := HashToken(expiredRaw)
	err = store.RegisterToken(&Token{
		ID:        "tok-exp",
		Hash:      expHash,
		Owner:     "charlie",
		Revoked:   false,
		ExpiresAt: time.Now().Add(-10 * time.Minute),
	})
	if err != nil {
		t.Fatalf("failed registering expired token: %v", err)
	}

	tests := []struct {
		name      string
		rawToken  string
		wantOwner string
		wantErr   error
	}{
		{
			name:      "Valid registered token",
			rawToken:  validRaw,
			wantOwner: "alice",
			wantErr:   nil,
		},
		{
			name:      "Missing empty token",
			rawToken:  "",
			wantOwner: "",
			wantErr:   ErrMissingToken,
		},
		{
			name:      "Unknown token",
			rawToken:  "pt_nonexistent_token_999",
			wantOwner: "",
			wantErr:   ErrInvalidToken,
		},
		{
			name:      "Revoked token",
			rawToken:  revokedRaw,
			wantOwner: "",
			wantErr:   ErrTokenRevoked,
		},
		{
			name:      "Expired token",
			rawToken:  expiredRaw,
			wantOwner: "",
			wantErr:   ErrTokenExpired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := store.ValidateToken(tt.rawToken)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ValidateToken(%q) error = %v, wantErr %v", tt.rawToken, err, tt.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("ValidateToken(%q) unexpected error: %v", tt.rawToken, err)
				}
				if token.Owner != tt.wantOwner {
					t.Errorf("token owner = %q, want %q", token.Owner, tt.wantOwner)
				}
			}
		})
	}
}

func TestPreProvisionedTokensAndListing(t *testing.T) {
	store := NewMemoryTokenStore()
	preTokens := []string{"pt_env_token_1", "pt_env_token_2", "pt_env_token_3"}

	if err := store.LoadPreProvisionedTokens(preTokens); err != nil {
		t.Fatalf("LoadPreProvisionedTokens error: %v", err)
	}

	list := store.ListTokens()
	if len(list) != len(preTokens) {
		t.Fatalf("expected %d tokens, got %d", len(preTokens), len(list))
	}

	for _, pt := range preTokens {
		if _, err := store.ValidateToken(pt); err != nil {
			t.Errorf("expected pre-provisioned token %q to be valid: %v", pt, err)
		}
	}
}

func TestConcurrentStoreAccess(t *testing.T) {
	store := NewMemoryTokenStore()
	const numGoroutines = 10
	const numOps = 50

	token, _ := store.RegisterRawToken("pt_concurrent_token", "user", 10, 100)

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				_, _ = store.ValidateToken("pt_concurrent_token")
				_ = store.ListTokens()
			}
		}(i)
	}
	wg.Wait()

	if token == nil {
		t.Fatal("token is nil")
	}
}

func TestAuditorLogging(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(buf, nil))
	auditor := NewAuditor(logger)

	auditor.LogSuccess("client-1", "127.0.0.1:5000", "myapp", "alice", "tok-123")
	auditor.LogFailure("client-2", "127.0.0.1:5001", "badapp", "invalid_token", ErrInvalidToken)

	out := buf.String()
	if !strings.Contains(out, "auth_success") {
		t.Errorf("expected output to contain auth_success")
	}
	if !strings.Contains(out, "auth_failure") {
		t.Errorf("expected output to contain auth_failure")
	}
}
