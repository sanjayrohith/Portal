package edge

import (
	"crypto/tls"
	"net"
	"net/http"
	"strings"
)

// VHostRouter extracts target subdomain identities from HTTP Host headers and TLS SNI indicators.
type VHostRouter struct {
	baseDomain string
}

// NewVHostRouter creates a new VHostRouter with the configured apex/base domain (e.g. "portal.dev").
func NewVHostRouter(baseDomain string) *VHostRouter {
	base := strings.ToLower(strings.TrimSpace(baseDomain))
	base = strings.TrimPrefix(base, ".")
	return &VHostRouter{
		baseDomain: base,
	}
}

// BaseDomain returns the configured base domain.
func (r *VHostRouter) BaseDomain() string {
	return r.baseDomain
}

// ExtractSubdomainFromHost extracts the subdomain from an HTTP Host header (e.g. "foo.portal.dev:8080" -> "foo").
func (r *VHostRouter) ExtractSubdomainFromHost(hostHeader string) string {
	hostHeader = strings.TrimSpace(hostHeader)
	if hostHeader == "" {
		return ""
	}

	// Strip optional port
	host, _, err := net.SplitHostPort(hostHeader)
	if err != nil {
		host = hostHeader
	}
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))

	if r.baseDomain == "" {
		return ""
	}

	// Exact match to baseDomain means apex domain, not a subdomain
	if host == r.baseDomain {
		return ""
	}

	suffix := "." + r.baseDomain
	if strings.HasSuffix(host, suffix) {
		sub := strings.TrimSuffix(host, suffix)
		// Only single-level subdomain supported or first label if nested
		parts := strings.Split(sub, ".")
		return parts[len(parts)-1]
	}

	return ""
}

// ExtractSubdomainFromRequest extracts the target subdomain from an http.Request.
// It checks TLS ClientHello ServerName if available, falling back to Host header.
func (r *VHostRouter) ExtractSubdomainFromRequest(req *http.Request) string {
	if req == nil {
		return ""
	}

	// Check SNI from TLS handshake if present
	if req.TLS != nil && req.TLS.ServerName != "" {
		if sub := r.ExtractSubdomainFromHost(req.TLS.ServerName); sub != "" {
			return sub
		}
	}

	// Fallback to Host header
	host := req.Host
	if host == "" && req.URL != nil {
		host = req.URL.Host
	}
	return r.ExtractSubdomainFromHost(host)
}

// ExtractSubdomainFromClientHello extracts the target subdomain from a TLS ClientHelloInfo SNI.
func (r *VHostRouter) ExtractSubdomainFromClientHello(chi *tls.ClientHelloInfo) string {
	if chi == nil || chi.ServerName == "" {
		return ""
	}
	return r.ExtractSubdomainFromHost(chi.ServerName)
}
