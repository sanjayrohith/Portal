package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteRepository implements SubdomainRepository backed by an embedded SQLite database.
type SQLiteRepository struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewSQLiteRepository opens or creates an embedded SQLite database and runs auto-migrations.
func NewSQLiteRepository(dsn string) (*SQLiteRepository, error) {
	if dsn == "" {
		dsn = "portal.db"
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database %s: %w", dsn, err)
	}

	// Optimize SQLite for single-process concurrency
	db.SetMaxOpenConns(1)

	repo := &SQLiteRepository{db: db}
	if err := repo.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite migration failed: %w", err)
	}

	return repo, nil
}

func (r *SQLiteRepository) migrate() error {
	query := `
	CREATE TABLE IF NOT EXISTS subdomain_reservations (
		subdomain TEXT PRIMARY KEY,
		token_hash TEXT NOT NULL,
		owner TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		last_active_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_subdomain_token_hash ON subdomain_reservations(token_hash);
	`
	_, err := r.db.Exec(query)
	return err
}

// ClaimReservation atomically reserves a subdomain or reclaims it if owned by the same token.
func (r *SQLiteRepository) ClaimReservation(ctx context.Context, res *SubdomainReservation) (*SubdomainReservation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var existingTokenHash, owner string
	var createdAt, lastActiveAt time.Time

	row := r.db.QueryRowContext(ctx, `
		SELECT token_hash, owner, created_at, last_active_at
		FROM subdomain_reservations WHERE subdomain = ?
	`, res.Subdomain)

	err := row.Scan(&existingTokenHash, &owner, &createdAt, &lastActiveAt)
	if err == sql.ErrNoRows {
		// Insert new reservation
		now := time.Now()
		if res.CreatedAt.IsZero() {
			res.CreatedAt = now
		}
		if res.LastActiveAt.IsZero() {
			res.LastActiveAt = now
		}

		_, err := r.db.ExecContext(ctx, `
			INSERT INTO subdomain_reservations (subdomain, token_hash, owner, created_at, last_active_at)
			VALUES (?, ?, ?, ?, ?)
		`, res.Subdomain, res.TokenHash, res.Owner, res.CreatedAt, res.LastActiveAt)
		if err != nil {
			return nil, fmt.Errorf("failed to insert reservation: %w", err)
		}
		return res, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to query reservation: %w", err)
	}

	// Subdomain exists: verify token ownership
	if existingTokenHash != res.TokenHash {
		return nil, ErrReservationConflict
	}

	// Update last_active_at
	now := time.Now()
	_, err = r.db.ExecContext(ctx, `
		UPDATE subdomain_reservations SET last_active_at = ? WHERE subdomain = ?
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
func (r *SQLiteRepository) GetReservation(ctx context.Context, subdomain string) (*SubdomainReservation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var res SubdomainReservation
	row := r.db.QueryRowContext(ctx, `
		SELECT subdomain, token_hash, owner, created_at, last_active_at
		FROM subdomain_reservations WHERE subdomain = ?
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
func (r *SQLiteRepository) ListReservations(ctx context.Context) ([]*SubdomainReservation, error) {
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
func (r *SQLiteRepository) UpdateLastActive(ctx context.Context, subdomain string, lastActive time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	res, err := r.db.ExecContext(ctx, `
		UPDATE subdomain_reservations SET last_active_at = ? WHERE subdomain = ?
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

// DeleteReservation deletes a reservation.
func (r *SQLiteRepository) DeleteReservation(ctx context.Context, subdomain string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	res, err := r.db.ExecContext(ctx, `DELETE FROM subdomain_reservations WHERE subdomain = ?`, subdomain)
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
func (r *SQLiteRepository) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.db.Close()
}
