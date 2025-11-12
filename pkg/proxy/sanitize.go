package proxy

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

// SanitizeForwardedHeaders overwrites spoofable forwarding headers with
// values derived from the verified connection state, appends the peer to
// X-Forwarded-For, and strips hop-by-hop headers (including any header named
// by the Connection token list). It must run on every request before the
// request is forwarded to the local upstream.
func SanitizeForwardedHeaders(header http.Header, originalHost, proto, peerIP string) {
	if header == nil {
		return
	}
	if originalHost != "" {
		header.Set("X-Forwarded-Host", originalHost)
	}
	if proto == "" {
		proto = "http"
	}
	header.Set("X-Forwarded-Proto", proto)
	if peerIP != "" {
		if prior := header.Get("X-Forwarded-For"); prior != "" {
			header.Set("X-Forwarded-For", prior+", "+peerIP)
		} else {
			header.Set("X-Forwarded-For", peerIP)
		}
	}
	stripConnectionTokens(header)
	stripHopByHopHeaders(header)
}

// ValidateProxyRequest rejects requests carrying CR/LF bytes in the request
// line or header values, blocking HTTP response-splitting and log/header
// injection through the tunnel.
func ValidateProxyRequest(req *http.Request) error {
	if req == nil {
		return fmt.Errorf("cannot validate nil request")
	}
	if strings.ContainsAny(req.Method, "\r\n") {
		return fmt.Errorf("invalid request method %q", req.Method)
	}
	if err := ValidateHeaderValue(req.Host, "Host"); err != nil {
		return err
	}
	if req.URL != nil {
		if err := ValidateHeaderValue(req.URL.Host, "URL host"); err != nil {
			return err
		}
		for _, part := range []struct {
			label string
			value string
		}{
			{"URL path", req.URL.Path},
			{"URL query", req.URL.RawQuery},
		} {
			if strings.ContainsAny(part.value, "\r\n") {
				return fmt.Errorf("invalid %s: contains CR or LF", part.label)
			}
		}
	}
	for name, values := range req.Header {
		if err := ValidateHeaderValue(name, "header name"); err != nil {
			return err
		}
		for _, value := range values {
			if err := ValidateHeaderValue(value, "header "+name); err != nil {
				return err
			}
		}
	}
	return nil
}

// ValidateHeaderValue rejects a single header name or value containing CR/LF.
func ValidateHeaderValue(value, label string) error {
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("invalid %s: contains CR or LF", label)
	}
	return nil
}

// PeerIPFromAddr extracts the host portion of a peer address for
// X-Forwarded-For. Unparseable input yields "" (header left untouched).
func PeerIPFromAddr(peerAddr string) string {
	peerAddr = strings.TrimSpace(peerAddr)
	if peerAddr == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(peerAddr); err == nil {
		return host
	}
	return peerAddr
}

// stripConnectionTokens deletes every header named by the Connection header
// token list (RFC 9110: those headers are per-hop and must not be forwarded).
func stripConnectionTokens(header http.Header) {
	for _, token := range strings.Split(header.Get("Connection"), ",") {
		if name := strings.TrimSpace(token); name != "" {
			header.Del(name)
		}
	}
}
