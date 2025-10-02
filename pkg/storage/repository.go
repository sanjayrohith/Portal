package storage

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrReservationNotFound is returned when a requested subdomain reservation does not exist.
	ErrReservationNotFound = errors.New("subdomain reservation not found")
	// ErrReservationConflict is returned when attempting to claim a subdomain owned by another token.
	ErrReservationConflict = errors.New("subdomain already reserved by another token")
)

// SubdomainReservation represents a persistent claim of a subdomain by an authenticated token.
type SubdomainReservation struct {
	Subdomain    string    `json:"subdomain"`
	TokenHash    string    `json:"token_hash"`
	Owner        string    `json:"owner"`
	CreatedAt    time.Time `json:"created_at"`
	LastActiveAt time.Time `json:"last_active_at"`
}

// SubdomainRepository defines the abstract persistence interface for subdomain reservations.
type SubdomainRepository interface {
	// ClaimReservation atomically reserves a subdomain for a token, or reclaims it if owned by the same token.
	ClaimReservation(ctx context.Context, res *SubdomainReservation) (*SubdomainReservation, error)

	// GetReservation retrieves a reservation by its subdomain.
	GetReservation(ctx context.Context, subdomain string) (*SubdomainReservation, error)

	// ListReservations returns all persistent reservations.
	ListReservations(ctx context.Context) ([]*SubdomainReservation, error)

	// UpdateLastActive updates the last active timestamp for a subdomain reservation.
	UpdateLastActive(ctx context.Context, subdomain string, lastActive time.Time) error

	// DeleteReservation removes a persistent reservation.
	DeleteReservation(ctx context.Context, subdomain string) error

	// Close closes the underlying repository storage connections.
	Close() error
}
