package edge

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sanjayrohith/portal/pkg/mux"
	"github.com/sanjayrohith/portal/pkg/proxy"
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

	t.Run("Active subdomain routes successfully over multiplexed stream", func(t *testing.T) {
		// Mock session
		serverConn, clientConn := net.Pipe()
		defer serverConn.Close()
		defer clientConn.Close()

		serverSession := mux.NewSession(serverConn, true)
		clientSession := mux.NewSession(clientConn, false)
		defer serverSession.Close()
		defer clientSession.Close()

		// Mock client-side tunnel accepting stream and returning HTTP response
		go func() {
			stream, err := clientSession.AcceptStream()
			if err != nil {
				return
			}
			defer stream.Close()

			req, err := proxy.ReadHTTPRequest(stream)
			if err != nil {
				return
			}

			resp := &http.Response{
				StatusCode: http.StatusOK,
				ProtoMajor: 1,
				ProtoMinor: 1,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewBufferString("hello from tunnel client")),
			}
			resp.Header.Set("Content-Type", "text/plain")
			_ = proxy.WriteHTTPResponse(stream, resp)
			_ = req
		}()

		_, err := reg.Allocate("myblog", "tok-hash-123", "owner1", serverSession)
		if err != nil {
			t.Fatalf("failed to allocate subdomain: %v", err)
		}

		req, _ := http.NewRequest("GET", "http://myblog.tunnel.portal.dev/posts", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d (%s)", rec.Code, rec.Body.String())
		}
		if rec.Header().Get("X-Portal-Routed") != "true" {
			t.Errorf("expected X-Portal-Routed header")
		}
		if rec.Body.String() != "hello from tunnel client" {
			t.Errorf("expected body 'hello from tunnel client', got %q", rec.Body.String())
		}
	})
}
