package registry

import (
	"strings"
	"sync"

	"github.com/sanjayrohith/portal/pkg/mux"
)

// SubdomainRegistry provides thread-safe in-memory routing and mapping from subdomains to active sessions.
type SubdomainRegistry struct {
	mu       sync.RWMutex
	records  map[string]*SubdomainRecord
	sessions map[*mux.Session]string
}

// NewSubdomainRegistry creates an empty SubdomainRegistry.
func NewSubdomainRegistry() *SubdomainRegistry {
	return &SubdomainRegistry{
		records:  make(map[string]*SubdomainRecord),
		sessions: make(map[*mux.Session]string),
	}
}

// GetSession looks up the currently active multiplexer session for a subdomain.
func (r *SubdomainRegistry) GetSession(subdomain string) (*mux.Session, bool) {
	subdomain = strings.ToLower(strings.TrimSpace(subdomain))

	r.mu.RLock()
	defer r.mu.RUnlock()

	record, ok := r.records[subdomain]
	if !ok || record.ActiveSession == nil || record.ActiveSession.IsClosed() {
		return nil, false
	}

	return record.ActiveSession, true
}

// GetRecord returns a snapshot copy of a subdomain record.
func (r *SubdomainRegistry) GetRecord(subdomain string) (*SubdomainRecord, bool) {
	subdomain = strings.ToLower(strings.TrimSpace(subdomain))

	r.mu.RLock()
	defer r.mu.RUnlock()

	record, ok := r.records[subdomain]
	if !ok {
		return nil, false
	}

	cp := *record
	return &cp, true
}

// ActiveTunnelsCount returns the count of currently active connected tunnels.
func (r *SubdomainRegistry) ActiveTunnelsCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	active := 0
	for _, rec := range r.records {
		if rec.ActiveSession != nil && !rec.ActiveSession.IsClosed() {
			active++
		}
	}
	return active
}

// ListActiveSubdomains returns a slice of all subdomains with active connections.
func (r *SubdomainRegistry) ListActiveSubdomains() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []string
	for name, rec := range r.records {
		if rec.ActiveSession != nil && !rec.ActiveSession.IsClosed() {
			list = append(list, name)
		}
	}
	return list
}

// IsSubdomainAvailable checks whether a subdomain can be claimed by a token.
func (r *SubdomainRegistry) IsSubdomainAvailable(subdomain, tokenHash string) (bool, error) {
	normalized, err := ValidateSubdomain(subdomain)
	if err != nil {
		return false, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	rec, exists := r.records[normalized]
	if !exists {
		return true, nil
	}

	// Available if previously reserved by the exact same token
	return rec.TokenHash == tokenHash, nil
}
