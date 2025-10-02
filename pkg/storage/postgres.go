package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "github.com/lib/pq"
)

// PostgresRepository implements SubdomainRepository backed by PostgreSQL for cluster control planes.
type PostgresRepository struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewPostgresRepository opens a connection to PostgreSQL and executes auto-migrations.
func NewPostgresRepository(dsn string) (*PostgresRepository, error) {
	if dsn == "" {
		return nil, fmt.Errorf("postgres DSN cannot be empty")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	repo := &PostgresRepository{db: db}
	return repo, nil
}

// InitSchema creates tables and indices if not present.
func (r *PostgresRepository) InitSchema(ctx context.Context) error {
	query := `
	CREATE TABLE IF NOT EXISTS subdomain_reservations (
		subdomain VARCHAR(63) PRIMARY KEY,
		token_hash VARCHAR(128) NOT NULL,
		owner VARCHAR(255) NOT NULL,
		created_at TIMESTAMPTZ NOT NULL,
		last_active_at TIMESTAMPTZ NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_pg_subdomain_token_hash ON subdomain_reservations(token_hash);
	`
	_, err := r.db.ExecContext(ctx, query)
	return err
}

// ClaimReservation reserves or reclaims a subdomain atomically in Postgres.
func (r *PostgresRepository) ClaimReservation(ctx context.Context, res *SubdomainReservation) (*SubdomainReservation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var existingTokenHash, owner string
	var createdAt, lastActiveAt time.Time

	row := r.db.QueryRowContext(ctx, `
		SELECT token_hash, owner, created_at, last_active_at
		FROM subdomain_reservations WHERE subdomain = $1
	`, res.Subdomain)

	err := row.Scan(&existingTokenHash, &owner, &createdAt, &lastActiveAt)
	if err == sql.ErrNoRows {
		now := time.Now()
		if res.CreatedAt.IsZero() {
			res.CreatedAt = now
		}
		if res.LastActiveAt.IsZero() {
			res.LastActiveAt = now
		}

		_, err := r.db.ExecContext(ctx, `
			INSERT INTO subdomain_reservations (subdomain, token_hash, owner, created_at, last_active_at)
			VALUES ($1, $2, $3, $4, $5)
		`, res.Subdomain, res.TokenHash, res.Owner, res.CreatedAt, res.LastActiveAt)
		if err != nil {
			return nil, fmt.Errorf("failed to insert reservation: %w", err)
		}
		return res, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to query reservation: %w", err)
	}

	if existingTokenHash != res.TokenHash {
		return nil, ErrReservationConflict
	}

	now := time.Now()
	_, err = r.db.ExecContext(ctx, `
		UPDATE subdomain_reservations SET last_active_at = $1 WHERE subdomain = $2
	`, now, res.Subdomain)
	if err != nil {
		return nil, fmt.Errorf("failed to update last_active: %w", err)
	}

	return &SubdomainReservation{
		Subdomain:    res.Subdomain,
		TokenHash:    existingTokenHash,
		Owner:        owner,
		CreatedAt:    createdAt,
		LastActiveAt: now,
	}, nil
}

// GetReservation retrieves a reservation by subdomain.
func (r *PostgresRepository) GetReservation(ctx context.Context, subdomain string) (*SubdomainReservation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var res SubdomainReservation
	row := r.db.QueryRowContext(ctx, `
		SELECT subdomain, token_hash, owner, created_at, last_active_at
		FROM subdomain_reservations WHERE subdomain = $1
	`, subdomain)

	err := row.Scan(&res.Subdomain, &res.TokenHash, &res.Owner, &res.CreatedAt, &res.LastActiveAt)
	if err == sql.ErrNoRows {
		return nil, ErrReservationNotFound
	} else if err != nil {
		return nil, fmt.Errorf("failed to get reservation: %w", err)
	}

	return &res, nil
}

// ListReservations returns all reservations.
func (r *PostgresRepository) ListReservations(ctx context.Context) ([]*SubdomainReservation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rows, err := r.db.QueryContext(ctx, `
		SELECT subdomain, token_hash, owner, created_at, last_active_at
		FROM subdomain_reservations
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list reservations: %w", err)
	}
	defer rows.Close()

	var list []*SubdomainReservation
	for rows.Next() {
		var res SubdomainReservation
		if err := rows.Scan(&res.Subdomain, &res.TokenHash, &res.Owner, &res.CreatedAt, &res.LastActiveAt); err != nil {
			return nil, err
		}
		list = append(list, &res)
	}
	return list, rows.Err()
}

// UpdateLastActive updates the active timestamp.
func (r *PostgresRepository) UpdateLastActive(ctx context.Context, subdomain string, lastActive time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	res, err := r.db.ExecContext(ctx, `
		UPDATE subdomain_reservations SET last_active_at = $1 WHERE subdomain = $2
	`, lastActive, subdomain)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrReservationNotFound
	}
	return nil
}

// DeleteReservation removes a persistent reservation.
func (r *PostgresRepository) DeleteReservation(ctx context.Context, subdomain string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	res, err := r.db.ExecContext(ctx, `DELETE FROM subdomain_reservations WHERE subdomain = $1`, subdomain)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrReservationNotFound
	}
	return nil
}

// Close closes the database connection.
func (r *PostgresRepository) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.db.Close()
}
