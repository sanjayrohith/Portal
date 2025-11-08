package edge

import (
	"crypto/tls"
	"net/http"
	"testing"
)

func TestVHostRouter_ExtractSubdomain(t *testing.T) {
	router := NewVHostRouter("portal.dev")

	tests := []struct {
		name    string
		host    string
		wantSub string
	}{
		{"standard subdomain", "myapp.portal.dev", "myapp"},
		{"subdomain with port", "api.portal.dev:8080", "api"},
		{"apex domain", "portal.dev", ""},
		{"apex with port", "portal.dev:443", ""},
		{"unrelated domain", "otherdomain.com", ""},
		{"empty host", "", ""},
		{"uppercase host", "TEST-APP.PORTAL.DEV:443", "test-app"},
		{"trailing dot", "demo.portal.dev.", "demo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := router.ExtractSubdomainFromHost(tt.host)
			if got != tt.wantSub {
				t.Errorf("ExtractSubdomainFromHost(%q) = %q, want %q", tt.host, got, tt.wantSub)
			}
		})
	}

	// Test from http.Request
	req, _ := http.NewRequest("GET", "http://docs.portal.dev/path", nil)
	if sub := router.ExtractSubdomainFromRequest(req); sub != "docs" {
		t.Errorf("expected docs, got %s", sub)
	}

	// Test with TLS SNI taking precedence
	req.TLS = &tls.ConnectionState{
		ServerName: "secure.portal.dev",
	}
	if sub := router.ExtractSubdomainFromRequest(req); sub != "secure" {
		t.Errorf("expected secure from SNI, got %s", sub)
	}

	// Test TLS ClientHelloInfo
	chi := &tls.ClientHelloInfo{
		ServerName: "sni.portal.dev",
	}
	if sub := router.ExtractSubdomainFromClientHello(chi); sub != "sni" {
		t.Errorf("expected sni from ClientHelloInfo, got %s", sub)
	}
}
