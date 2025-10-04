package control

import (
	"net"
	"testing"
	"time"

	"github.com/sanjayrohith/portal/pkg/auth"
	portalErr "github.com/sanjayrohith/portal/pkg/errors"
	"github.com/sanjayrohith/portal/pkg/registry"
)

func TestPersistentSubdomainReclamationHandshake(t *testing.T) {
	tokenStore := auth.NewMemoryTokenStore()
	_, _ = tokenStore.RegisterRawToken("alice-prod-tok", "alice", 5, 100)
	_, _ = tokenStore.RegisterRawToken("bob-prod-tok", "bob", 5, 100)

	reg := registry.NewSubdomainRegistry()
	authorizer := NewPersistentReclamationAuthorizer(tokenStore, reg, "tunnel.portal.dev", nil)

	// 1. Alice connects and claims "analytics"
	c1, s1 := net.Pipe()
	defer c1.Close()
	defer s1.Close()

	go func() {
		_, _, _ = ServerHandshake(s1, authorizer, 2*time.Second)
	}()

	helloAlice := ClientHello{
		ClientID:  "client-alice-1",
		AuthToken: "alice-prod-tok",
		Subdomain: "analytics",
	}

	respAlice, err := ClientHandshake(c1, helloAlice, 2*time.Second)
	if err != nil {
		t.Fatalf("Alice initial handshake failed: %v", err)
	}
	if respAlice.AssignedSubdomain != "analytics" {
		t.Errorf("expected dashboard, got %s", respAlice.AssignedSubdomain)
	}

	// 2. Bob attempts to usurp "analytics" during handshake -> rejected with 409
	c2, s2 := net.Pipe()
	defer c2.Close()
	defer s2.Close()

	go func() {
		_, _, _ = ServerHandshake(s2, authorizer, 2*time.Second)
	}()

	helloBob := ClientHello{
		ClientID:  "client-bob-1",
		AuthToken: "bob-prod-tok",
		Subdomain: "analytics",
	}

	_, err = ClientHandshake(c2, helloBob, 2*time.Second)
	if err == nil {
		t.Fatalf("expected Bob's handshake to fail on taken subdomain")
	}
	if !portalErr.IsSubdomainTaken(err) {
		t.Errorf("expected StatusSubdomainTaken, got %v", err)
	}

	// 3. Alice reconnects on a new socket with the same token and reclaims "analytics"
	c3, s3 := net.Pipe()
	defer c3.Close()
	defer s3.Close()

	go func() {
		_, _, _ = ServerHandshake(s3, authorizer, 2*time.Second)
	}()

	helloAliceReconnect := ClientHello{
		ClientID:  "client-alice-2",
		AuthToken: "alice-prod-tok",
		Subdomain: "analytics",
	}

	respAliceReclaim, err := ClientHandshake(c3, helloAliceReconnect, 2*time.Second)
	if err != nil {
		t.Fatalf("Alice reconnect failed: %v", err)
	}
	if respAliceReclaim.AssignedSubdomain != "analytics" {
		t.Errorf("expected Alice to reclaim 'dashboard', got %s", respAliceReclaim.AssignedSubdomain)
	}
}
