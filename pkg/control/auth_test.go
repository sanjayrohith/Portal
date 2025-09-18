package control

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/sanjayrohith/portal/pkg/auth"
	portalErr "github.com/sanjayrohith/portal/pkg/errors"
)

func TestHandshakeAuthenticationFlow(t *testing.T) {
	store := auth.NewMemoryTokenStore()
	validToken := "pt_handshake_secret_token_123"
	_, err := store.RegisterRawToken(validToken, "developer-sam", 5, 100)
	if err != nil {
		t.Fatalf("failed registering token: %v", err)
	}

	authorizer := NewTokenAuthorizer(store, "portal.example.com")

	// Case 1: Valid authentication token
	t.Run("Valid Token", func(t *testing.T) {
		clientConn, serverConn := net.Pipe()
		defer clientConn.Close()
		defer serverConn.Close()

		go func() {
			_, _, _ = ServerHandshake(serverConn, authorizer, 2*time.Second)
		}()

		hello := ClientHello{
			Version:   CurrentProtocolVersion,
			ClientID:  "client-1",
			AuthToken: validToken,
			Subdomain: "myapi",
		}

		sHello, err := ClientHandshake(clientConn, hello, 2*time.Second)
		if err != nil {
			t.Fatalf("expected successful handshake, got error: %v", err)
		}
		if sHello.StatusCode != portalErr.StatusSuccess {
			t.Errorf("expected StatusSuccess, got %v", sHello.StatusCode)
		}
		if sHello.AssignedSubdomain != "myapi" {
			t.Errorf("expected assigned subdomain 'myapi', got %q", sHello.AssignedSubdomain)
		}
	})

	// Case 2: Missing authentication token
	t.Run("Missing Token Rejection", func(t *testing.T) {
		clientConn, serverConn := net.Pipe()
		defer clientConn.Close()
		defer serverConn.Close()

		go func() {
			_, _, _ = ServerHandshake(serverConn, authorizer, 2*time.Second)
		}()

		hello := ClientHello{
			Version:   CurrentProtocolVersion,
			ClientID:  "client-2",
			AuthToken: "", // Missing
			Subdomain: "myapi",
		}

		_, err := ClientHandshake(clientConn, hello, 2*time.Second)
		if err == nil {
			t.Fatalf("expected error on missing token, got nil")
		}
		if !portalErr.IsUnauthorized(err) {
			t.Errorf("expected IsUnauthorized(err) to be true, got %v", err)
		}
	})

	// Case 3: Invalid authentication token
	t.Run("Invalid Token Rejection", func(t *testing.T) {
		clientConn, serverConn := net.Pipe()
		defer clientConn.Close()
		defer serverConn.Close()

		go func() {
			_, _, _ = ServerHandshake(serverConn, authorizer, 2*time.Second)
		}()

		hello := ClientHello{
			Version:   CurrentProtocolVersion,
			ClientID:  "client-3",
			AuthToken: "pt_invalid_forged_token",
			Subdomain: "myapi",
		}

		_, err := ClientHandshake(clientConn, hello, 2*time.Second)
		if err == nil {
			t.Fatalf("expected error on invalid token, got nil")
		}
		if !portalErr.IsUnauthorized(err) {
			t.Errorf("expected IsUnauthorized(err) to be true, got %v", err)
		}
	})

	// Case 4: Revoked authentication token
	t.Run("Revoked Token Rejection", func(t *testing.T) {
		revokedToken := "pt_revoked_token_456"
		_, _ = store.RegisterRawToken(revokedToken, "developer-bob", 5, 100)
		_ = store.RevokeToken(revokedToken)

		clientConn, serverConn := net.Pipe()
		defer clientConn.Close()
		defer serverConn.Close()

		go func() {
			_, _, _ = ServerHandshake(serverConn, authorizer, 2*time.Second)
		}()

		hello := ClientHello{
			Version:   CurrentProtocolVersion,
			ClientID:  "client-4",
			AuthToken: revokedToken,
			Subdomain: "myapi",
		}

		_, err := ClientHandshake(clientConn, hello, 2*time.Second)
		if err == nil {
			t.Fatalf("expected error on revoked token, got nil")
		}
		if !portalErr.IsUnauthorized(err) {
			t.Errorf("expected IsUnauthorized(err) to be true, got %v", err)
		}
	})
}

var _ = errors.New
