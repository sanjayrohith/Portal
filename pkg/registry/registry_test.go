package registry

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	portalErr "github.com/sanjayrohith/portal/pkg/errors"
	"github.com/sanjayrohith/portal/pkg/mux"
)

func createMockSession() (*mux.Session, net.Conn) {
	c1, c2 := net.Pipe()
	sess := mux.NewSession(c1, false)
	return sess, c2
}

func TestSubdomainAllocationAndLookup(t *testing.T) {
	reg := NewSubdomainRegistry()
	sess, c2 := createMockSession()
	defer sess.Close()
	defer c2.Close()

	rec, err := reg.Allocate("myapi", "hash_alice", "alice", sess)
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}

	if rec.Subdomain != "myapi" {
		t.Errorf("expected subdomain 'myapi', got %q", rec.Subdomain)
	}
	if rec.Owner != "alice" {
		t.Errorf("expected owner 'alice', got %q", rec.Owner)
	}

	retrievedSess, ok := reg.GetSession("myapi")
	if !ok || retrievedSess != sess {
		t.Errorf("GetSession failed to retrieve active session")
	}

	if reg.ActiveTunnelsCount() != 1 {
		t.Errorf("expected active tunnels count 1, got %d", reg.ActiveTunnelsCount())
	}
}

func TestReclaimSubdomainAcrossReconnects(t *testing.T) {
	reg := NewSubdomainRegistry()

	sess1, c1 := createMockSession()
	defer c1.Close()

	_, err := reg.Allocate("stableapp", "hash_sam", "sam", sess1)
	if err != nil {
		t.Fatalf("first allocation failed: %v", err)
	}

	// Client reconnects with a fresh session
	sess2, c2 := createMockSession()
	defer sess2.Close()
	defer c2.Close()

	rec2, err := reg.Allocate("stableapp", "hash_sam", "sam", sess2)
	if err != nil {
		t.Fatalf("reclaiming subdomain failed: %v", err)
	}

	if rec2.ActiveSession != sess2 {
		t.Errorf("expected active session to be updated to sess2")
	}

	// Verify old session was superseded and closed
	if !sess1.IsClosed() {
		t.Errorf("expected previous session to be closed when superseded")
	}
}

func TestConflictResolutionUsurpationDenied(t *testing.T) {
	reg := NewSubdomainRegistry()

	sess1, c1 := createMockSession()
	defer sess1.Close()
	defer c1.Close()

	_, err := reg.Allocate("popular-name", "hash_alice", "alice", sess1)
	if err != nil {
		t.Fatalf("alice allocation failed: %v", err)
	}

	sess2, c2 := createMockSession()
	defer sess2.Close()
	defer c2.Close()

	// Bob tries to claim Alice's subdomain
	_, err = reg.Allocate("popular-name", "hash_bob", "bob", sess2)
	if err == nil {
		t.Fatalf("expected error on conflicting allocation, got nil")
	}

	if !errors.Is(err, portalErr.ErrSubdomainTaken) {
		t.Errorf("expected ErrSubdomainTaken, got: %v", err)
	}

	// Bob uses fallback
	recBob, err := reg.AllocateWithFallback("popular-name", "hash_bob", "bob", sess2, true)
	if err != nil {
		t.Fatalf("AllocateWithFallback failed for bob: %v", err)
	}

	if recBob.Subdomain == "popular-name" {
		t.Errorf("expected alternate subdomain for bob, got %q", recBob.Subdomain)
	}
	if recBob.Owner != "bob" {
		t.Errorf("expected bob to own alternate subdomain, got %q", recBob.Owner)
	}
}

func TestSessionUnregisterAndRelease(t *testing.T) {
	reg := NewSubdomainRegistry()
	sess, c2 := createMockSession()
	defer c2.Close()

	_, err := reg.Allocate("testsub", "hash_user", "user", sess)
	if err != nil {
		t.Fatalf("allocate failed: %v", err)
	}

	// Unregister session on disconnect
	unregName, err := reg.UnregisterSession(sess)
	if err != nil {
		t.Fatalf("UnregisterSession error: %v", err)
	}
	if unregName != "testsub" {
		t.Errorf("expected unregistered name 'testsub', got %q", unregName)
	}

	// Record still exists (reservation retained)
	rec, exists := reg.GetRecord("testsub")
	if !exists || rec.ActiveSession != nil {
		t.Errorf("expected record retained with nil ActiveSession")
	}

	// Other user cannot release
	if err := reg.ReleaseSubdomain("testsub", "hash_impostor"); err == nil {
		t.Errorf("expected error when unauthorized user attempts release")
	}

	// Owning token releases reservation
	if err := reg.ReleaseSubdomain("testsub", "hash_user"); err != nil {
		t.Fatalf("ReleaseSubdomain error: %v", err)
	}

	if _, exists := reg.GetRecord("testsub"); exists {
		t.Errorf("expected record to be deleted after release")
	}
}

func TestConcurrentSubdomainContention(t *testing.T) {
	reg := NewSubdomainRegistry()
	const competitors = 50

	var successCount atomic.Int32
	var failureCount atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < competitors; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			tokenHash := fmt.Sprintf("token_hash_%d", id)
			owner := fmt.Sprintf("competitor_%d", id)

			sess, c := createMockSession()
			defer sess.Close()
			defer c.Close()

			_, err := reg.Allocate("contended-name", tokenHash, owner, sess)
			if err == nil {
				successCount.Add(1)
			} else if errors.Is(err, portalErr.ErrSubdomainTaken) {
				failureCount.Add(1)
			} else {
				t.Errorf("unexpected error on allocation: %v", err)
			}
		}(i)
	}

	wg.Wait()

	if successCount.Load() != 1 {
		t.Fatalf("expected exactly 1 winner, got %d", successCount.Load())
	}
	if failureCount.Load() != competitors-1 {
		t.Fatalf("expected %d rejections, got %d", competitors-1, failureCount.Load())
	}
}

func TestConcurrentDistinctAllocations(t *testing.T) {
	reg := NewSubdomainRegistry()
	const count = 40

	sessions := make([]*mux.Session, count)
	conns := make([]net.Conn, count)

	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		sess, c := createMockSession()
		sessions[i] = sess
		conns[i] = c

		wg.Add(1)
		go func(id int, s *mux.Session) {
			defer wg.Done()
			subdomain := fmt.Sprintf("sub-%d", id)
			tokenHash := fmt.Sprintf("token_%d", id)
			owner := fmt.Sprintf("user_%d", id)

			_, err := reg.Allocate(subdomain, tokenHash, owner, s)
			if err != nil {
				t.Errorf("unexpected error allocating %s: %v", subdomain, err)
			}
		}(i, sess)
	}
	wg.Wait()

	if reg.ActiveTunnelsCount() != count {
		t.Errorf("expected %d active tunnels, got %d", count, reg.ActiveTunnelsCount())
	}

	// Clean up all sessions
	for i := 0; i < count; i++ {
		_ = sessions[i].Close()
		_ = conns[i].Close()
	}
}

var _ = time.Now
