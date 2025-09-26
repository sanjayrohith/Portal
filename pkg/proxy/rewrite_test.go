package proxy

import (
	"net/http"
	"net/url"
	"testing"
)

func TestHostRewriter(t *testing.T) {
	t.Run("RewriteModePreserve", func(t *testing.T) {
		rewriter := NewHostRewriter(RewriteModePreserve, "", "")
		req, _ := http.NewRequest("GET", "http://example.com/test", nil)
		req.Host = "example.com"
		rewriter.RewriteRequest(req)

		if req.Host != "example.com" {
			t.Errorf("expected example.com, got %s", req.Host)
		}
		if req.Header.Get("X-Forwarded-Host") != "example.com" {
			t.Errorf("expected X-Forwarded-Host to be example.com, got %s", req.Header.Get("X-Forwarded-Host"))
		}
		if req.Header.Get("X-Forwarded-Proto") != "http" {
			t.Errorf("expected X-Forwarded-Proto to be http, got %s", req.Header.Get("X-Forwarded-Proto"))
		}
	})

	t.Run("RewriteModeLocalhost", func(t *testing.T) {
		rewriter := NewHostRewriter(RewriteModeLocalhost, "", "127.0.0.1:3000")
		req, _ := http.NewRequest("GET", "http://tunnel.portal.dev/test", nil)
		req.Host = "tunnel.portal.dev"
		rewriter.RewriteRequest(req)

		if req.Host != "127.0.0.1:3000" {
			t.Errorf("expected 127.0.0.1:3000, got %s", req.Host)
		}
		if req.Header.Get("X-Forwarded-Host") != "tunnel.portal.dev" {
			t.Errorf("expected X-Forwarded-Host tunnel.portal.dev, got %s", req.Header.Get("X-Forwarded-Host"))
		}
	})

	t.Run("RewriteModeCustom", func(t *testing.T) {
		rewriter := NewHostRewriter(RewriteModeCustom, "api.local.internal", "")
		req := &http.Request{
			Method: "GET",
			URL:    &url.URL{Path: "/"},
			Host:   "client.portal.io",
			Header: make(http.Header),
		}
		req.Header.Set("Connection", "keep-alive")
		rewriter.RewriteRequest(req)

		if req.Host != "api.local.internal" {
			t.Errorf("expected api.local.internal, got %s", req.Host)
		}
		if req.Header.Get("Connection") != "" {
			t.Errorf("expected Connection header to be stripped, got %s", req.Header.Get("Connection"))
		}
	})
}
