package registry

import (
	"fmt"
	"strings"
	"time"

	portalErr "github.com/sanjayrohith/portal/pkg/errors"
	"github.com/sanjayrohith/portal/pkg/mux"
)

// UnregisterSession detaches an active session on client disconnect while preserving the subdomain reservation.
func (r *SubdomainRegistry) UnregisterSession(session *mux.Session) (string, error) {
	if session == nil {
		return "", nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	subdomain, exists := r.sessions[session]
	if !exists {
		return "", nil
	}

	delete(r.sessions, session)

	if rec, ok := r.records[subdomain]; ok {
		if rec.ActiveSession == session {
			rec.ActiveSession = nil
			rec.LastActiveAt = time.Now()
		}
	}

	return subdomain, nil
}

// ReleaseSubdomain explicitly deletes a persistent reservation if requested by its owning token.
func (r *SubdomainRegistry) ReleaseSubdomain(subdomain, tokenHash string) error {
	normalized, err := ValidateSubdomain(subdomain)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	rec, exists := r.records[normalized]
	if !exists {
		return nil
	}

	if rec.TokenHash != tokenHash {
		return fmt.Errorf("%w: cannot release subdomain owned by another token", portalErr.ErrForbidden)
	}

	if rec.ActiveSession != nil {
		delete(r.sessions, rec.ActiveSession)
		_ = rec.ActiveSession.Close()
	}

	delete(r.records, normalized)
	return nil
}

// PurgeInactiveReservations deletes reservations that have had no active session for longer than maxAge.
func (r *SubdomainRegistry) PurgeInactiveReservations(maxAge time.Duration) int {
	if maxAge <= 0 {
		return 0
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	purged := 0

	for name, rec := range r.records {
		if rec.ActiveSession == nil || rec.ActiveSession.IsClosed() {
			if now.Sub(rec.LastActiveAt) > maxAge {
				delete(r.records, name)
				purged++
			}
		}
	}

	return purged
}

var _ = strings.ToLower
