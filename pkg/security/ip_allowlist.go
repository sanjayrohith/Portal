package security

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

// IPAllowlist matches IP addresses against configured CIDR blocks and IP addresses.
type IPAllowlist struct {
	networks []*net.IPNet
	ips      map[string]bool
}

// NewIPAllowlist parses a list of CIDR subnets or individual IP addresses.
func NewIPAllowlist(entries []string) (*IPAllowlist, error) {
	al := &IPAllowlist{
		networks: make([]*net.IPNet, 0),
		ips:      make(map[string]bool),
	}

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		// Check if CIDR notation
		if strings.Contains(entry, "/") {
			_, ipNet, err := net.ParseCIDR(entry)
			if err != nil {
				return nil, fmt.Errorf("invalid CIDR %q: %w", entry, err)
			}
			al.networks = append(al.networks, ipNet)
		} else {
			parsed := net.ParseIP(entry)
			if parsed == nil {
				return nil, fmt.Errorf("invalid IP address %q", entry)
			}
			al.ips[parsed.String()] = true
		}
	}

	return al, nil
}

// IsAllowed returns true if the specified IP matches the allowlist.
func (al *IPAllowlist) IsAllowed(ip net.IP) bool {
	if ip == nil {
		return false
	}

	if al.ips[ip.String()] {
		return true
	}

	for _, network := range al.networks {
		if network.Contains(ip) {
			return true
		}
	}

	return false
}

// ExtractClientIP extracts the client IP address from the request.
// If trustProxy is true, X-Forwarded-For or X-Real-IP is checked first.
func ExtractClientIP(r *http.Request, trustProxy bool) net.IP {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			clientIPStr := strings.TrimSpace(parts[0])
			if ip := net.ParseIP(clientIPStr); ip != nil {
				return ip
			}
		}
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			if ip := net.ParseIP(strings.TrimSpace(xri)); ip != nil {
				return ip
			}
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return net.ParseIP(host)
	}

	return net.ParseIP(r.RemoteAddr)
}

// IPAllowlistMiddleware returns an HTTP middleware enforcing that requests originate
// from an allowed IP address or CIDR range.
func IPAllowlistMiddleware(allowlist *IPAllowlist, trustProxy bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientIP := ExtractClientIP(r, trustProxy)
			if clientIP == nil || !allowlist.IsAllowed(clientIP) {
				http.Error(w, "Forbidden: IP not allowed", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
