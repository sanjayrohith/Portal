package edge

import (
	"fmt"
	"io"
	"net/http"

	"github.com/sanjayrohith/portal/pkg/proxy"
	"github.com/sanjayrohith/portal/pkg/registry"
)

// ProxyHandler implements http.Handler routing incoming edge requests to active tunnel sessions
// by dispatching each incoming HTTP request as a concurrent multiplexed stream.
type ProxyHandler struct {
	router   *VHostRouter
	registry *registry.SubdomainRegistry
}

// NewProxyHandler creates a new ProxyHandler.
func NewProxyHandler(router *VHostRouter, reg *registry.SubdomainRegistry) *ProxyHandler {
	return &ProxyHandler{
		router:   router,
		registry: reg,
	}
}

// ServeHTTP inspects the virtual host, checks registry, opens a multiplexed stream to the client,
// forwards the HTTP request, and streams back the HTTP response.
func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	subdomain := h.router.ExtractSubdomainFromRequest(r)
	if subdomain == "" {
		RenderNotFound(w, "", h.router.BaseDomain(), "Missing or unrecognized subdomain")
		return
	}

	session, ok := h.registry.GetSession(subdomain)
	if !ok || session == nil {
		RenderNotFound(w, subdomain, h.router.BaseDomain(), "No active tunnel connected for subdomain")
		return
	}

	// Open a distinct multiplexed stream on the tunnel session for this request
	stream, err := session.OpenStream()
	if err != nil {
		http.Error(w, fmt.Sprintf("503 Service Unavailable: failed to open stream: %v", err), http.StatusServiceUnavailable)
		return
	}
	defer stream.Close()

	// Forward the incoming HTTP request over the multiplexed stream
	if err := proxy.WriteHTTPRequest(stream, r); err != nil {
		http.Error(w, fmt.Sprintf("502 Bad Gateway: failed to forward request: %v", err), http.StatusBadGateway)
		return
	}

	// Read HTTP response from stream
	resp, err := proxy.ReadHTTPResponse(stream, r)
	if err != nil {
		http.Error(w, fmt.Sprintf("502 Bad Gateway: failed to read upstream response: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.Header().Set("X-Portal-Routed", "true")
	w.WriteHeader(resp.StatusCode)

	// Stream response body back to the caller
	_, _ = io.Copy(w, resp.Body)
}
