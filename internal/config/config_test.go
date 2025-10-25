package config

import (
	"strings"
	"testing"
)

func TestClientConfigValidation(t *testing.T) {
	cfg := DefaultClientConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid default config, got error: %v", err)
	}

	cfg.InspectorAddr = "0.0.0.0:4040"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error when inspector binds to 0.0.0.0")
	}

	cfg.InspectorAddr = "127.0.0.1:4040"
	cfg.ServerAddr = ""
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error when ServerAddr is empty")
	}
}

func TestNormalizeLocalTarget(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		wantErr  bool
	}{
		{"3000", "127.0.0.1:3000", false},
		{":8080", "127.0.0.1:8080", false},
		{"localhost:5000", "127.0.0.1:5000", false},
		{"127.0.0.1:9000", "127.0.0.1:9000", false},
		{"invalid-port", "", true},
		{"70000", "", true},
		{"http://localhost:5000", "", true},
	}

	for _, tt := range tests {
		got, err := NormalizeLocalTarget(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("NormalizeLocalTarget(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && got != tt.expected {
			t.Errorf("NormalizeLocalTarget(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestClientConfigValidationHints(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ClientConfig)
		want   string
	}{
		{"server scheme", func(cfg *ClientConfig) { cfg.ServerAddr = "https://example.com:443" }, "URL scheme"},
		{"server port", func(cfg *ClientConfig) { cfg.ServerAddr = "example.com:not-a-port" }, "1 to 65535"},
		{"subdomain", func(cfg *ClientConfig) { cfg.Subdomain = "Bad_Name" }, "lowercase"},
		{"host header", func(cfg *ClientConfig) { cfg.HostHeader = "preserve" }, "rewrite"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := DefaultClientConfig()
			test.mutate(cfg)
			if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want text containing %q", err, test.want)
			}
		})
	}
}

func TestValidateSubdomain(t *testing.T) {
	for _, subdomain := range []string{"app", "app-2", "0demo"} {
		if err := ValidateSubdomain(subdomain); err != nil {
			t.Errorf("ValidateSubdomain(%q) = %v", subdomain, err)
		}
	}
	for _, subdomain := range []string{"Bad", "-app", "app-", "app_name", strings.Repeat("a", 64)} {
		if err := ValidateSubdomain(subdomain); err == nil {
			t.Errorf("ValidateSubdomain(%q) succeeded, want error", subdomain)
		}
	}
}

func TestServerConfigValidation(t *testing.T) {
	cfg := DefaultServerConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid default server config, got error: %v", err)
	}

	cfg.Domain = ""
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error on empty domain")
	}
}
