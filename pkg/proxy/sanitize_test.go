package proxy

import (
	"net/http"
	"testing"
)

func TestForwardedHeadersCannotBeSpoofed(t *testing.T) {
	rewriter := NewHostRewriter(RewriteModePreserve, "", "")
	req, _ := http.NewRequest("GET", "http://app.portal.test/", nil)
	req.Host = "app.portal.test"
	req.Header.Set("X-Forwarded-Host", "evil.example.com")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-For", "203.0.113.99")

	if err := rewriter.RewriteRequestFromPeer(req, "198.51.100.7:54321"); err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get("X-Forwarded-Host"); got != "app.portal.test" {
		t.Fatalf("X-Forwarded-Host = %q, spoofed value leaked through", got)
	}
	if got := req.Header.Get("X-Forwarded-Proto"); got != "http" {
		t.Fatalf("X-Forwarded-Proto = %q, want http", got)
	}
	want := "203.0.113.99, 198.51.100.7"
	if got := req.Header.Get("X-Forwarded-For"); got != want {
		t.Fatalf("X-Forwarded-For = %q, want %q", got, want)
	}
}

func TestConnectionTokensAreStripped(t *testing.T) {
	rewriter := NewHostRewriter(RewriteModePreserve, "", "")
	req, _ := http.NewRequest("GET", "http://app.portal.test/", nil)
	req.Host = "app.portal.test"
	req.Header.Set("Connection", "close, X-Custom-Hop")
	req.Header.Set("X-Custom-Hop", "must-not-forward")

	if err := rewriter.RewriteRequest(req); err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get("X-Custom-Hop"); got != "" {
		t.Fatalf("Connection-token header forwarded: %q", got)
	}
	if got := req.Header.Get("Connection"); got != "" {
		t.Fatalf("Connection header forwarded: %q", got)
	}
}

func TestCRLFInjectionRejected(t *testing.T) {
	rewriter := NewHostRewriter(RewriteModePreserve, "", "")

	injected := &http.Request{
		Method: "GET",
		Host:   "app.portal.test\r\nX-Injected: evil",
		Header: make(http.Header),
	}
	if err := rewriter.RewriteRequest(injected); err == nil {
		t.Fatal("CRLF in Host was accepted, want rejection")
	}

	headerInjection, _ := http.NewRequest("GET", "http://app.portal.test/", nil)
	headerInjection.Host = "app.portal.test"
	headerInjection.Header.Set("X-Note", "line1\r\nContent-Length: 999")
	if err := rewriter.RewriteRequest(headerInjection); err == nil {
		t.Fatal("CRLF in header value was accepted, want rejection")
	}

	if err := ValidateHeaderValue("clean-value_1.0", "label"); err != nil {
		t.Fatalf("clean value rejected: %v", err)
	}
}

func TestPeerIPFromAddr(t *testing.T) {
	if got := PeerIPFromAddr("198.51.100.7:54321"); got != "198.51.100.7" {
		t.Fatalf("peer IP = %q", got)
	}
	if got := PeerIPFromAddr(""); got != "" {
		t.Fatalf("empty peer should yield empty IP, got %q", got)
	}
}
