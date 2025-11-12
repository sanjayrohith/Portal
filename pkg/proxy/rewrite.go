package proxy

import (
	"fmt"
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
	mode       RewriteMode
	customHost string
	targetHost string
}

// NewHostRewriter creates a new HostRewriter.
func NewHostRewriter(mode RewriteMode, customHost, targetHost string) *HostRewriter {
	return &HostRewriter{
		mode:       mode,
		customHost: customHost,
		targetHost: targetHost,
	}
}

// RewriteRequest modifies the given http.Request in-place according to
// configuration. It is equivalent to RewriteRequestFromPeer with an unknown
// peer (no X-Forwarded-For appended).
func (r *HostRewriter) RewriteRequest(req *http.Request) error {
	return r.RewriteRequestFromPeer(req, "")
}

// RewriteRequestFromPeer rewrites req and sanitizes forwarding headers using
// peerAddr (typically the edge stream's RemoteAddr) as the trusted client
// identity. Client-supplied X-Forwarded-* values are always overwritten so a
// malicious client cannot spoof its origin to the upstream.
func (r *HostRewriter) RewriteRequestFromPeer(req *http.Request, peerAddr string) error {
	if req == nil {
		return fmt.Errorf("cannot rewrite nil request")
	}
	if err := ValidateProxyRequest(req); err != nil {
		return err
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

	proto := "http"
	if req.TLS != nil {
		proto = "https"
	}

	// Overwrite (never trust) forwarding headers, then strip hop-by-hop state.
	SanitizeForwardedHeaders(req.Header, originalHost, proto, PeerIPFromAddr(peerAddr))
	return nil
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
