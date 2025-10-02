package storage

import (
	"context"
	"testing"
)

func TestPostgresRepositoryConstructor(t *testing.T) {
	// Empty DSN should fail
	_, err := NewPostgresRepository("")
	if err == nil {
		t.Errorf("expected error for empty DSN")
	}

	// Valid format syntax constructor
	repo, err := NewPostgresRepository("postgres://user:pass@localhost:5432/portal_db?sslmode=disable")
	if err != nil {
		t.Fatalf("failed to construct postgres repo: %v", err)
	}
	defer repo.Close()

	// Verify contract conformance at compile time
	var _ SubdomainRepository = repo

	// In offline unit tests, attempting to connect or init schema without a live PG instance should return error
	ctx := context.Background()
	err = repo.InitSchema(ctx)
	if err == nil {
		t.Logf("PostgreSQL instance is running locally and initialized")
	} else {
		t.Logf("Expected connection error in offline environment: %v", err)
	}
}
