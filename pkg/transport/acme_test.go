package transport

import (
	"testing"
)

type testDNSProvider struct{}

func (testDNSProvider) Present(string, string, string) error { return nil }

func (testDNSProvider) CleanUp(string, string, string) error { return nil }

func TestNewACMEClientConfiguresWildcardDNS01(t *testing.T) {
	client, err := NewACMEClient(ACMEConfig{
		Email:        "admin@example.com",
		Domain:       "*.tunnel.example.com.",
		DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory",
	}, testDNSProvider{})
	if err != nil {
		t.Fatalf("NewACMEClient() error = %v", err)
	}
	if client.domain != "tunnel.example.com" {
		t.Fatalf("normalized domain = %q, want tunnel.example.com", client.domain)
	}
	if client.client == nil || client.client.Challenge == nil {
		t.Fatal("ACME client or DNS-01 challenge manager was nil")
	}
}

func TestNewACMEClientRejectsInvalidConfiguration(t *testing.T) {
	provider := testDNSProvider{}
	tests := []ACMEConfig{
		{Email: "admin@example.com", Domain: "localhost"},
		{Email: "admin@example.com", Domain: "example.com/path"},
		{Email: "", Domain: "example.com"},
	}
	for _, config := range tests {
		if _, err := NewACMEClient(config, provider); err == nil {
			t.Errorf("NewACMEClient(%+v) succeeded, want error", config)
		}
	}
	if _, err := NewACMEClient(ACMEConfig{Email: "admin@example.com", Domain: "example.com"}, nil); err == nil {
		t.Error("NewACMEClient() with nil provider succeeded, want error")
	}
}
