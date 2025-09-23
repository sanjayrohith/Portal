package registry

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	portalErr "github.com/sanjayrohith/portal/pkg/errors"
	"github.com/sanjayrohith/portal/pkg/mux"
)

// SubdomainConflictError captures contextual details when a requested subdomain is already claimed.
type SubdomainConflictError struct {
	Subdomain     string `json:"subdomain"`
	ReservedOwner string `json:"reserved_owner"`
	RequestedBy   string `json:"requested_by"`
}

func (e *SubdomainConflictError) Error() string {
	return fmt.Sprintf("subdomain %q is reserved by another token (claimed by %s, requested by %s)",
		e.Subdomain, e.ReservedOwner, e.RequestedBy)
}

func (e *SubdomainConflictError) Is(target error) bool {
	return errors.Is(target, portalErr.ErrSubdomainTaken)
}

// SuggestAlternative generates an available alternative subdomain when a collision occurs.
func (r *SubdomainRegistry) SuggestAlternative(base string) string {
	normalized, err := ValidateSubdomain(base)
	if err != nil {
		normalized = "tunnel"
	}

	for i := 1; i <= 20; i++ {
		candidate := fmt.Sprintf("%s-%d", normalized, i)
		r.mu.RLock()
		_, exists := r.records[candidate]
		r.mu.RUnlock()
		if !exists {
			return candidate
		}
	}

	// Suffix random hex suffix
	randBytes := make([]byte, 2)
	_, _ = rand.Read(randBytes)
	return fmt.Sprintf("%s-%s", normalized, hex.EncodeToString(randBytes))
}

// AllocateWithFallback attempts allocation of requestedSubdomain; if in conflict and allowFallback is true,
// it allocates a suggested non-conflicting alternative.
func (r *SubdomainRegistry) AllocateWithFallback(
	requestedSubdomain string,
	tokenHash string,
	owner string,
	session *mux.Session,
	allowFallback bool,
) (*SubdomainRecord, error) {
	rec, err := r.Allocate(requestedSubdomain, tokenHash, owner, session)
	if err == nil {
		return rec, nil
	}

	if errors.Is(err, portalErr.ErrSubdomainTaken) && allowFallback {
		alternative := r.SuggestAlternative(requestedSubdomain)
		return r.Allocate(alternative, tokenHash, owner, session)
	}

	return nil, err
}
