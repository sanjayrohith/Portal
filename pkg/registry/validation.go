package registry

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/sanjayrohith/portal/pkg/errors"
	"github.com/sanjayrohith/portal/pkg/mux"
)

// RFC 1123 DNS label regex: lowercase alphanumeric and hyphens, 1-63 chars, no leading/trailing hyphen.
var rfc1123Regex = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// ReservedSubdomains lists system subdomains that cannot be claimed by tunnel clients.
var ReservedSubdomains = map[string]struct{}{
	"admin":     {},
	"api":       {},
	"app":       {},
	"auth":      {},
	"dashboard": {},
	"dev":       {},
	"dns":       {},
	"ftp":       {},
	"healthz":   {},
	"internal":  {},
	"localhost": {},
	"mail":      {},
	"metrics":   {},
	"mx":        {},
	"portal":    {},
	"portald":   {},
	"readyz":    {},
	"root":      {},
	"smtp":      {},
	"ssl":       {},
	"status":    {},
	"test":      {},
	"tunnel":    {},
	"tunnels":   {},
	"vpn":       {},
	"web":       {},
	"www":       {},
}

// SubdomainRecord represents a persistent or active subdomain allocation.
type SubdomainRecord struct {
	Subdomain     string       `json:"subdomain"`
	TokenHash     string       `json:"token_hash"`
	Owner         string       `json:"owner"`
	ActiveSession *mux.Session `json:"-"`
	CreatedAt     time.Time    `json:"created_at"`
	LastActiveAt  time.Time    `json:"last_active_at"`
}

// ValidateSubdomain validates the subdomain string according to RFC 1123 and reserved list.
func ValidateSubdomain(subdomain string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(subdomain))
	if name == "" {
		return "", fmt.Errorf("%w: subdomain cannot be empty", errors.ErrSubdomainInvalid)
	}

	if len(name) < 1 || len(name) > 63 {
		return "", fmt.Errorf("%w: length must be between 1 and 63 characters, got %d", errors.ErrSubdomainInvalid, len(name))
	}

	if !rfc1123Regex.MatchString(name) {
		return "", fmt.Errorf("%w: %q must match RFC 1123 (lowercase alphanumeric, hyphens allowed only internally)", errors.ErrSubdomainInvalid, name)
	}

	if _, reserved := ReservedSubdomains[name]; reserved {
		return "", fmt.Errorf("%w: %q is a reserved system subdomain", errors.ErrSubdomainInvalid, name)
	}

	return name, nil
}
