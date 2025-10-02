package storage

import (
	"context"
	"testing"
	"time"
)

// MockSubdomainRepository provides an in-memory implementation of SubdomainRepository for contract verification.
type MockSubdomainRepository struct {
	data map[string]*SubdomainReservation
}

func NewMockSubdomainRepository() *MockSubdomainRepository {
	return &MockSubdomainRepository{
		data: make(map[string]*SubdomainReservation),
	}
}

func (m *MockSubdomainRepository) ClaimReservation(ctx context.Context, res *SubdomainReservation) (*SubdomainReservation, error) {
	existing, ok := m.data[res.Subdomain]
	if !ok {
		cp := *res
		if cp.CreatedAt.IsZero() {
			cp.CreatedAt = time.Now()
		}
		if cp.LastActiveAt.IsZero() {
			cp.LastActiveAt = cp.CreatedAt
		}
		m.data[res.Subdomain] = &cp
		return &cp, nil
	}

	if existing.TokenHash != res.TokenHash {
		return nil, ErrReservationConflict
	}

	existing.LastActiveAt = time.Now()
	return existing, nil
}

func (m *MockSubdomainRepository) GetReservation(ctx context.Context, subdomain string) (*SubdomainReservation, error) {
	res, ok := m.data[subdomain]
	if !ok {
		return nil, ErrReservationNotFound
	}
	cp := *res
	return &cp, nil
}

func (m *MockSubdomainRepository) ListReservations(ctx context.Context) ([]*SubdomainReservation, error) {
	var list []*SubdomainReservation
	for _, v := range m.data {
		cp := *v
		list = append(list, &cp)
	}
	return list, nil
}

func (m *MockSubdomainRepository) UpdateLastActive(ctx context.Context, subdomain string, lastActive time.Time) error {
	res, ok := m.data[subdomain]
	if !ok {
		return ErrReservationNotFound
	}
	res.LastActiveAt = lastActive
	return nil
}

func (m *MockSubdomainRepository) DeleteReservation(ctx context.Context, subdomain string) error {
	if _, ok := m.data[subdomain]; !ok {
		return ErrReservationNotFound
	}
	delete(m.data, subdomain)
	return nil
}

func (m *MockSubdomainRepository) Close() error {
	return nil
}

func TestSubdomainRepositoryContract(t *testing.T) {
	repo := NewMockSubdomainRepository()
	ctx := context.Background()

	res := &SubdomainReservation{
		Subdomain: "api",
		TokenHash: "hash-token-1",
		Owner:     "team-alpha",
	}

	claimed, err := repo.ClaimReservation(ctx, res)
	if err != nil {
		t.Fatalf("ClaimReservation failed: %v", err)
	}
	if claimed.Subdomain != "api" {
		t.Errorf("expected api, got %s", claimed.Subdomain)
	}

	// Reclaim with same token succeeds
	_, err = repo.ClaimReservation(ctx, res)
	if err != nil {
		t.Fatalf("reclaiming with same token failed: %v", err)
	}

	// Usurp with different token fails
	diffRes := &SubdomainReservation{
		Subdomain: "api",
		TokenHash: "hash-token-2",
		Owner:     "team-beta",
	}
	_, err = repo.ClaimReservation(ctx, diffRes)
	if err != ErrReservationConflict {
		t.Fatalf("expected ErrReservationConflict, got %v", err)
	}
}
