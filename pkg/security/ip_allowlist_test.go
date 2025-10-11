package security

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIPAllowlist_IsAllowed(t *testing.T) {
	al, err := NewIPAllowlist([]string{
		"192.168.1.0/24",
		"10.0.0.1",
		"2001:db8::/32",
	})
	if err != nil {
		t.Fatalf("unexpected error creating IPAllowlist: %v", err)
	}

	tests := []struct {
		ip      string
		allowed bool
	}{
		{"192.168.1.1", true},
		{"192.168.1.254", true},
		{"192.168.2.1", false},
		{"10.0.0.1", true},
		{"10.0.0.2", false},
		{"2001:db8::1", true},
		{"2001:db9::1", false},
		{"invalid-ip", false},
	}

	for _, tt := range tests {
		ip := net.ParseIP(tt.ip)
		if got := al.IsAllowed(ip); got != tt.allowed {
			t.Errorf("IsAllowed(%q) = %v; want %v", tt.ip, got, tt.allowed)
		}
	}
}

func TestIPAllowlist_InvalidCIDR(t *testing.T) {
	_, err := NewIPAllowlist([]string{"invalid-cidr/999"})
	if err == nil {
		t.Errorf("expected error for invalid CIDR")
	}

	_, err = NewIPAllowlist([]string{"not-an-ip"})
	if err == nil {
		t.Errorf("expected error for invalid IP")
	}
}

func TestIPAllowlistMiddleware(t *testing.T) {
	al, err := NewIPAllowlist([]string{"192.168.1.0/24", "10.0.0.5"})
	if err != nil {
		t.Fatalf("failed to create allowlist: %v", err)
	}

	mw := IPAllowlistMiddleware(al, true)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	// Case 1: Allowed RemoteAddr
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.50:12345"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Code)
	}

	// Case 2: Disallowed RemoteAddr
	req = httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "172.16.0.1:12345"
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", rr.Code)
	}

	// Case 3: Trusted X-Forwarded-For allowed
	req = httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "172.16.0.1:12345" // Untrusted proxy addr
	req.Header.Set("X-Forwarded-For", "10.0.0.5, 172.16.0.1")
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK via X-Forwarded-For, got %d", rr.Code)
	}

	// Case 4: Untrusted proxy (trustProxy=false) ignores header
	mwNoProxy := IPAllowlistMiddleware(al, false)
	handlerNoProxy := mwNoProxy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req = httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "172.16.0.1:12345"
	req.Header.Set("X-Forwarded-For", "10.0.0.5")
	rr = httptest.NewRecorder()
	handlerNoProxy.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden when trustProxy=false, got %d", rr.Code)
	}
}
