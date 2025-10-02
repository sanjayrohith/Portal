package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteRepository(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_portal.db")

	repo, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create sqlite repo: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()

	// 1. Claim subdomain
	res := &SubdomainReservation{
		Subdomain: "my-app",
		TokenHash: "hash-secret-token",
		Owner:     "alice",
	}

	saved, err := repo.ClaimReservation(ctx, res)
	if err != nil {
		t.Fatalf("ClaimReservation failed: %v", err)
	}
	if saved.Subdomain != "my-app" {
		t.Errorf("expected my-app, got %s", saved.Subdomain)
	}

	// 2. Query record
	got, err := repo.GetReservation(ctx, "my-app")
	if err != nil {
		t.Fatalf("GetReservation failed: %v", err)
	}
	if got.Owner != "alice" || got.TokenHash != "hash-secret-token" {
		t.Errorf("unexpected record: %+v", got)
	}

	// 3. Reclaim with same token succeeds
	_, err = repo.ClaimReservation(ctx, res)
	if err != nil {
		t.Fatalf("reclaim with same token failed: %v", err)
	}

	// 4. Usurpation with different token fails
	conflictRes := &SubdomainReservation{
		Subdomain: "my-app",
		TokenHash: "different-token-hash",
		Owner:     "bob",
	}
	_, err = repo.ClaimReservation(ctx, conflictRes)
	if err != ErrReservationConflict {
		t.Fatalf("expected ErrReservationConflict, got %v", err)
	}

	// 5. List reservations
	list, err := repo.ListReservations(ctx)
	if err != nil {
		t.Fatalf("ListReservations failed: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 reservation, got %d", len(list))
	}

	// 6. Update last active
	newTime := time.Now().Add(5 * time.Minute)
	if err := repo.UpdateLastActive(ctx, "my-app", newTime); err != nil {
		t.Fatalf("UpdateLastActive failed: %v", err)
	}

	// 7. Delete reservation
	if err := repo.DeleteReservation(ctx, "my-app"); err != nil {
		t.Fatalf("DeleteReservation failed: %v", err)
	}

	_, err = repo.GetReservation(ctx, "my-app")
	if err != ErrReservationNotFound {
		t.Fatalf("expected ErrReservationNotFound, got %v", err)
	}

	_ = os.Remove(dbPath)
}
