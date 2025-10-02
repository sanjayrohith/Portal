package storage_test

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"testing"

	portalErr "github.com/sanjayrohith/portal/pkg/errors"
	"github.com/sanjayrohith/portal/pkg/mux"
	"github.com/sanjayrohith/portal/pkg/registry"
	"github.com/sanjayrohith/portal/pkg/storage"
)

func createTestSession() (*mux.Session, net.Conn) {
	c1, c2 := net.Pipe()
	return mux.NewSession(c1, true), c2
}

func TestReservationPersistenceAcrossRestart(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "daemon_portal.db")
	ctx := context.Background()

	// --- STEP 1: First daemon lifetime ---
	storage1, err := storage.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("first storage.NewSQLiteRepository failed: %v", err)
	}

	reg1 := registry.NewSubdomainRegistryWithStorage(storage1)
	if err := reg1.LoadFromStorage(ctx); err != nil {
		t.Fatalf("reg1 LoadFromStorage failed: %v", err)
	}

	sess1, c1 := createTestSession()
	defer c1.Close()

	// Alice allocates "customer-dashboard"
	rec, err := reg1.Allocate("customer-dashboard", "token-alice-hash", "alice", sess1)
	if err != nil {
		t.Fatalf("reg1 Allocate failed: %v", err)
	}
	if rec.Subdomain != "customer-dashboard" {
		t.Errorf("expected customer-dashboard, got %s", rec.Subdomain)
	}

	// Daemon shutdown / crash: close storage and session
	_ = sess1.Close()
	if err := storage1.Close(); err != nil {
		t.Fatalf("storage1 Close failed: %v", err)
	}

	// --- STEP 2: Second daemon lifetime (restarting daemon from same DB) ---
	storage2, err := storage.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("second storage.NewSQLiteRepository failed: %v", err)
	}
	defer storage2.Close()

	reg2 := registry.NewSubdomainRegistryWithStorage(storage2)
	// Restore state from disk
	if err := reg2.LoadFromStorage(ctx); err != nil {
		t.Fatalf("reg2 LoadFromStorage failed: %v", err)
	}

	// Verify "customer-dashboard" is present in registry without active session
	loadedRec, ok := reg2.GetRecord("customer-dashboard")
	if !ok {
		t.Fatalf("expected customer-dashboard to be restored in reg2")
	}
	if loadedRec.Owner != "alice" || loadedRec.TokenHash != "token-alice-hash" {
		t.Errorf("restored record corrupted: %+v", loadedRec)
	}
	if loadedRec.ActiveSession != nil {
		t.Errorf("expected no active session on restored record")
	}

	// --- STEP 3: Ownership usurpation test ---
	sessEve, cEve := createTestSession()
	defer sessEve.Close()
	defer cEve.Close()

	// Eve attempts to usurp Alice's reserved subdomain
	_, err = reg2.Allocate("customer-dashboard", "token-eve-hash", "eve", sessEve)
	if err == nil {
		t.Fatalf("expected usurpation attempt by Eve to fail with conflict error")
	}
	if !errors.Is(err, portalErr.ErrSubdomainTaken) {
		t.Errorf("expected ErrSubdomainTaken, got: %v", err)
	}

	// --- STEP 4: Legitimate owner reconnect test ---
	sessAliceNew, cAliceNew := createTestSession()
	defer sessAliceNew.Close()
	defer cAliceNew.Close()

	// Alice reconnects with new session and claims back her subdomain
	reclaimedRec, err := reg2.Allocate("customer-dashboard", "token-alice-hash", "alice", sessAliceNew)
	if err != nil {
		t.Fatalf("legitimate reclaim by Alice failed: %v", err)
	}
	if reclaimedRec.ActiveSession != sessAliceNew {
		t.Errorf("active session was not updated on reclaimed record")
	}

	// Check active session lookup
	activeSess, ok := reg2.GetSession("customer-dashboard")
	if !ok || activeSess != sessAliceNew {
		t.Errorf("GetSession failed to return new active session for reclaimed subdomain")
	}
}

func errorsAs(err error, target any) bool {
	if err == nil {
		return false
	}
	type causer interface {
		Error() string
	}
	return true
}
