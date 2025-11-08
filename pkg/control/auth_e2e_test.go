package control

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/sanjayrohith/portal/pkg/auth"
	portalErr "github.com/sanjayrohith/portal/pkg/errors"
	"github.com/sanjayrohith/portal/pkg/security"
)

func runHandshake(t *testing.T, authorizer HandshakeAuthorizer, hello ClientHello) (*ServerHello, error, error) {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	serverResult := make(chan error, 1)
	go func() {
		_, _, err := ServerHandshake(serverConn, authorizer, time.Second)
		serverResult <- err
	}()

	clientHello, clientErr := ClientHandshake(clientConn, hello, time.Second)
	serverErr := <-serverResult
	return clientHello, clientErr, serverErr
}

func TestEndToEndTunnelAuthorizationAndQuotaEnforcement(t *testing.T) {
	store := auth.NewMemoryTokenStore()
	validRaw := "e2e-valid-token"
	_, err := store.RegisterRawToken(validRaw, "e2e-owner", 1, 100)
	if err != nil {
		t.Fatalf("register valid token: %v", err)
	}

	expiredRaw := "e2e-expired-token"
	if err := store.RegisterToken(&auth.Token{
		ID:        "expired-e2e",
		Hash:      auth.HashToken(expiredRaw),
		Owner:     "expired-owner",
		ExpiresAt: time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("register expired token: %v", err)
	}

	revokedRaw := "e2e-revoked-token"
	if _, err := store.RegisterRawToken(revokedRaw, "revoked-owner", 1, 100); err != nil {
		t.Fatalf("register revoked token: %v", err)
	}
	if err := store.RevokeToken(revokedRaw); err != nil {
		t.Fatalf("revoke token: %v", err)
	}

	quota := security.NewQuotaManager(1)
	baseAuthorizer := NewTokenAuthorizer(store, "tunnel.portal.dev")
	quotaAuthorizer := func(hello *ClientHello) (*ServerHello, error) {
		serverHello, err := baseAuthorizer(hello)
		if err != nil {
			return serverHello, err
		}
		token, err := store.ValidateToken(hello.AuthToken)
		if err != nil {
			return nil, err
		}
		if err := quota.AcquireTunnel(token.Hash, token.MaxTunnels); err != nil {
			return &ServerHello{
				Version:      CurrentProtocolVersion,
				StatusCode:   portalErr.StatusQuotaExceeded,
				ErrorMessage: err.Error(),
				ServerTime:   time.Now().Unix(),
			}, err
		}
		return serverHello, nil
	}

	newHello := func(token, clientID string) ClientHello {
		return ClientHello{
			Version:   CurrentProtocolVersion,
			ClientID:  clientID,
			AuthToken: token,
			Subdomain: "secure-app",
		}
	}

	t.Run("valid tunnel creation", func(t *testing.T) {
		response, clientErr, serverErr := runHandshake(t, quotaAuthorizer, newHello(validRaw, "valid-client"))
		if clientErr != nil || serverErr != nil {
			t.Fatalf("valid handshake failed: client=%v server=%v", clientErr, serverErr)
		}
		if response.StatusCode != portalErr.StatusSuccess {
			t.Fatalf("expected success, got %v", response.StatusCode)
		}
		if response.AssignedSubdomain != "secure-app" {
			t.Fatalf("expected secure-app, got %q", response.AssignedSubdomain)
		}
	})

	t.Run("second tunnel exceeds token quota", func(t *testing.T) {
		response, clientErr, serverErr := runHandshake(t, quotaAuthorizer, newHello(validRaw, "quota-client"))
		if clientErr == nil || serverErr == nil {
			t.Fatal("expected quota rejection")
		}
		if response == nil || response.StatusCode != portalErr.StatusQuotaExceeded {
			t.Fatalf("expected quota status, got response=%v error=%v", response, clientErr)
		}
		if !errors.Is(serverErr, portalErr.ErrQuotaExceeded) {
			t.Fatalf("expected ErrQuotaExceeded, got %v", serverErr)
		}
	})

	tests := []struct {
		name      string
		rawToken  string
		wantCause error
	}{
		{name: "forged token", rawToken: "e2e-forged-token", wantCause: auth.ErrInvalidToken},
		{name: "expired token", rawToken: expiredRaw, wantCause: auth.ErrTokenExpired},
		{name: "revoked token", rawToken: revokedRaw, wantCause: auth.ErrTokenRevoked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, clientErr, serverErr := runHandshake(t, quotaAuthorizer, newHello(tt.rawToken, tt.name+"-client"))
			if clientErr == nil || serverErr == nil {
				t.Fatal("expected unauthorized handshake rejection")
			}
			if response == nil || response.StatusCode != portalErr.StatusUnauthorized {
				t.Fatalf("expected unauthorized status, got response=%v error=%v", response, clientErr)
			}
			if !portalErr.IsUnauthorized(clientErr) {
				t.Fatalf("expected unauthorized error, got %v", clientErr)
			}
			if !errors.Is(serverErr, tt.wantCause) {
				t.Fatalf("expected cause %v, got %v", tt.wantCause, serverErr)
			}
		})
	}
}
