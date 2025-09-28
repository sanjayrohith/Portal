package edge

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sanjayrohith/portal/pkg/mux"
	"github.com/sanjayrohith/portal/pkg/registry"
)

func TestEdgeProxyRoutingAndFallback(t *testing.T) {
	reg := registry.NewSubdomainRegistry()
	router := NewVHostRouter("tunnel.portal.dev")
	handler := NewProxyHandler(router, reg)

	t.Run("Apex request returns 404 fallback", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "http://tunnel.portal.dev/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "Tunnel Not Online") {
			t.Errorf("expected fallback html in response")
		}
	})

	t.Run("Unallocated subdomain returns 404 fallback", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "http://ghost.tunnel.portal.dev/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "ghost.tunnel.portal.dev") {
			t.Errorf("expected subdomain mentioned in body")
		}
	})

	t.Run("Active subdomain routes successfully", func(t *testing.T) {
		// Mock session
		c1, c2 := net.Pipe()
		defer c1.Close()
		defer c2.Close()
		session := mux.NewSession(c1, true)
		defer session.Close()

		_, err := reg.Allocate("myblog", "tok-hash-123", "owner1", session)
		if err != nil {
			t.Fatalf("failed to allocate subdomain: %v", err)
		}

		req, _ := http.NewRequest("GET", "http://myblog.tunnel.portal.dev/posts", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", rec.Code)
		}
		if rec.Header().Get("X-Portal-Routed") != "true" {
			t.Errorf("expected X-Portal-Routed header")
		}
	})
}
