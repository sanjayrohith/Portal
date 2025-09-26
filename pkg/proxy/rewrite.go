package proxy

import (
	"net/http"
)

// RewriteMode defines how the Host header should be rewritten.
type RewriteMode int

const (
	// RewriteModePreserve preserves the incoming Host header as-is.
	RewriteModePreserve RewriteMode = iota
	// RewriteModeLocalhost rewrites the Host header to localhost or target host.
	RewriteModeLocalhost
	// RewriteModeCustom rewrites the Host header to a specified custom hostname.
	RewriteModeCustom
)

// HostRewriter handles rewriting the HTTP Host header and adding proxy forwarding headers.
type HostRewriter struct {
	mode        RewriteMode
	customHost  string
	targetHost  string
}

// NewHostRewriter creates a new HostRewriter.
func NewHostRewriter(mode RewriteMode, customHost, targetHost string) *HostRewriter {
	return &HostRewriter{
		mode:       mode,
		customHost: customHost,
		targetHost: targetHost,
	}
}

// RewriteRequest modifies the given http.Request in-place according to configuration.
func (r *HostRewriter) RewriteRequest(req *http.Request) {
	if req == nil {
		return
	}

	originalHost := req.Host
	if originalHost == "" && req.URL != nil {
		originalHost = req.URL.Host
	}

	switch r.mode {
	case RewriteModeLocalhost:
		if r.targetHost != "" {
			req.Host = r.targetHost
		} else {
			req.Host = "localhost"
		}
	case RewriteModeCustom:
		if r.customHost != "" {
			req.Host = r.customHost
		}
	case RewriteModePreserve:
		// Keep original Host
	}

	// Update URL.Host if URL is present
	if req.URL != nil {
		req.URL.Host = req.Host
	}

	// Set standard X-Forwarded-* headers if not already set
	if req.Header.Get("X-Forwarded-Host") == "" && originalHost != "" {
		req.Header.Set("X-Forwarded-Host", originalHost)
	}

	if req.Header.Get("X-Forwarded-Proto") == "" {
		proto := "http"
		if req.TLS != nil {
			proto = "https"
		}
		req.Header.Set("X-Forwarded-Proto", proto)
	}

	// Strip hop-by-hop headers for standard HTTP proxying
	stripHopByHopHeaders(req.Header)
}

// Hop-by-hop headers according to RFC 2616 Section 13.5.1
var hopByHopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"TE",
	"Trailers",
	"Transfer-Encoding",
	"Upgrade",
}

func stripHopByHopHeaders(header http.Header) {
	for _, h := range hopByHopHeaders {
		header.Del(h)
	}
}
