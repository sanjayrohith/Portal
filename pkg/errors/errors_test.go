package errors

import (
	"errors"
	"strings"
	"testing"
)

func TestStatusCodeString(t *testing.T) {
	tests := []struct {
		code     StatusCode
		expected string
	}{
		{StatusSuccess, "OK"},
		{StatusUnauthorized, "Unauthorized"},
		{StatusForbidden, "Forbidden"},
		{StatusNotFound, "Tunnel Not Found"},
		{StatusSubdomainTaken, "Subdomain Already Reserved"},
		{StatusSubdomainInvalid, "Invalid Subdomain Format"},
		{StatusRateLimited, "Rate Limit Exceeded"},
		{StatusQuotaExceeded, "Max Concurrent Tunnels Quota Exceeded"},
		{StatusInternalError, "Internal Server Error"},
		{StatusBadGateway, "Bad Gateway"},
		{StatusGatewayTimeout, "Gateway Timeout"},
		{StatusCode(999), "StatusCode(999)"},
	}

	for _, tt := range tests {
		got := tt.code.String()
		if got != tt.expected {
			t.Errorf("StatusCode(%d).String() = %q, want %q", tt.code, got, tt.expected)
		}
	}
}

func TestPortalErrorFormattingAndUnwrap(t *testing.T) {
	cause := ErrSubdomainTaken
	pe := New(StatusSubdomainTaken, "cannot reserve 'myapp'", cause)

	if !strings.Contains(pe.Error(), "Subdomain Already Reserved") {
		t.Errorf("expected error string to contain status description, got %q", pe.Error())
	}
	if !strings.Contains(pe.Error(), "cannot reserve 'myapp'") {
		t.Errorf("expected error string to contain message, got %q", pe.Error())
	}

	if !errors.Is(pe, ErrSubdomainTaken) {
		t.Errorf("errors.Is failed to match wrapped cause")
	}

	unwrapped := errors.Unwrap(pe)
	if unwrapped != cause {
		t.Errorf("expected unwrapped error %v, got %v", cause, unwrapped)
	}
}

func TestHelperPredicates(t *testing.T) {
	unauthErr := New(StatusUnauthorized, "token rejected", ErrUnauthorized)
	if !IsUnauthorized(unauthErr) {
		t.Errorf("expected IsUnauthorized to return true")
	}
	if !IsUnauthorized(ErrUnauthorized) {
		t.Errorf("expected IsUnauthorized(ErrUnauthorized) to return true")
	}
	if IsUnauthorized(ErrSubdomainTaken) {
		t.Errorf("expected IsUnauthorized to return false for subdomain error")
	}

	takenErr := New(StatusSubdomainTaken, "subdomain reserved", ErrSubdomainTaken)
	if !IsSubdomainTaken(takenErr) {
		t.Errorf("expected IsSubdomainTaken to return true")
	}

	rateErr := New(StatusRateLimited, "too many requests", ErrRateLimited)
	if !IsRateLimited(rateErr) {
		t.Errorf("expected IsRateLimited to return true")
	}
}
