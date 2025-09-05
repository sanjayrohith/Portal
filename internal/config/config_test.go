package config

import (
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
