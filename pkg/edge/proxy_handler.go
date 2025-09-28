package edge

import (
	"net/http"

	"github.com/sanjayrohith/portal/pkg/registry"
)

// ProxyHandler implements http.Handler routing incoming edge requests to active tunnel sessions.
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

// ServeHTTP inspects the virtual host, checks registry, and renders 404 if no session is active.
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

	// Session is active (dispatching to stream will be wired in Phase 9)
	w.Header().Set("X-Portal-Routed", "true")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("portal stream ready"))
}
