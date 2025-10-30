package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTempConfig(t *testing.T, contents string, ext string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "portald"+ext)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadServerConfigYAML(t *testing.T) {
	path := writeTempConfig(t, "domain: tunnel.example.com\ncontrol_addr: :9443\nstorage_type: sqlite\nrate_limit_requests_per_sec: 50\nmax_concurrent_tunnels: 3\nidle_timeout: 45s\nauth_tokens:\n  - pt_secret\nlog_level: debug\n", ".yaml")
	cfg, err := LoadServerConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Domain != "tunnel.example.com" || cfg.ControlAddr != ":9443" {
		t.Fatalf("unexpected addrs: %#v", cfg)
	}
	if cfg.RateLimitRequestsPerSec != 50 || cfg.MaxConcurrentTunnels != 3 {
		t.Fatalf("limits not applied: %#v", cfg)
	}
	if cfg.IdleTimeout != 45*time.Second {
		t.Fatalf("idle timeout = %s", cfg.IdleTimeout)
	}
	if len(cfg.AuthTokens) != 1 || cfg.AuthTokens[0] != "pt_secret" {
		t.Fatalf("tokens not loaded: %#v", cfg.AuthTokens)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestLoadServerConfigJSON(t *testing.T) {
	path := writeTempConfig(t, `{"domain":"json.example.com","rate_limit_requests_per_sec":10,"max_concurrent_tunnels":2,"max_request_body_size":1024,"idle_timeout":"30s"}`, ".json")
	cfg, err := LoadServerConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Domain != "json.example.com" || cfg.RateLimitRequestsPerSec != 10 {
		t.Fatalf("unexpected cfg: %#v", cfg)
	}
}

func TestLoadServerConfigDefaultsAndValidation(t *testing.T) {
	path := writeTempConfig(t, "domain: kept.example.com\n", ".yaml")
	cfg, err := LoadServerConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	def := DefaultServerConfig()
	if cfg.ControlAddr != def.ControlAddr || cfg.RateLimitRequestsPerSec != def.RateLimitRequestsPerSec {
		t.Fatalf("defaults not preserved: %#v", cfg)
	}

	bad := writeTempConfig(t, "domain: \"\"\nrate_limit_requests_per_sec: -1\n", ".yaml")
	if _, err := LoadServerConfig(bad); err == nil {
		t.Fatal("expected validation error for bad config")
	}
	if _, err := LoadServerConfig(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadServerConfigIfExists(t *testing.T) {
	cfg, err := LoadServerConfigIfExists(filepath.Join(t.TempDir(), "absent.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Domain != DefaultServerConfig().Domain {
		t.Fatal("absent file should yield defaults")
	}
}

func TestSaveServerConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "portald.yaml")
	cfg := DefaultServerConfig()
	cfg.Domain = "roundtrip.example.com"
	cfg.AuthTokens = []string{"pt_one"}
	if err := SaveServerConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadServerConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Domain != cfg.Domain || len(loaded.AuthTokens) != 1 {
		t.Fatalf("round trip mismatch: %#v", loaded)
	}
	if err := SaveServerConfig("", cfg); err == nil {
		t.Fatal("expected error for empty path")
	}
}
