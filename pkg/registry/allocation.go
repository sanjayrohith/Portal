package registry

import (
	"fmt"
	"time"

	"github.com/sanjayrohith/portal/pkg/errors"
	"github.com/sanjayrohith/portal/pkg/mux"
)

// Allocate claims or reclaims a subdomain for an authenticated token and active session.
func (r *SubdomainRegistry) Allocate(subdomain string, tokenHash string, owner string, session *mux.Session) (*SubdomainRecord, error) {
	if tokenHash == "" {
		return nil, fmt.Errorf("token hash required for allocation")
	}

	normalized, err := ValidateSubdomain(subdomain)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	rec, exists := r.records[normalized]
	if !exists {
		// First-come, first-served allocation
		now := time.Now()
		rec = &SubdomainRecord{
			Subdomain:     normalized,
			TokenHash:     tokenHash,
			Owner:         owner,
			ActiveSession: session,
			CreatedAt:     now,
			LastActiveAt:  now,
		}
		r.records[normalized] = rec
		if session != nil {
			r.sessions[session] = normalized
		}
		return rec, nil
	}

	// Subdomain is already claimed. Check if owned by the same token.
	if rec.TokenHash == tokenHash {
		// Reclaiming persistent subdomain across reconnects
		if rec.ActiveSession != nil && rec.ActiveSession != session && !rec.ActiveSession.IsClosed() {
			// Supersede previous session
			delete(r.sessions, rec.ActiveSession)
			_ = rec.ActiveSession.Close()
		}

		rec.ActiveSession = session
		rec.LastActiveAt = time.Now()
		if session != nil {
			r.sessions[session] = normalized
		}
		return rec, nil
	}

	// Claimed by a different token: handled by conflict resolution
	return nil, fmt.Errorf("%w: subdomain %q is reserved by another user", errors.ErrSubdomainTaken, normalized)
}
