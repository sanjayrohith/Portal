package config

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestManagerReloadSwapsConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "portald.yaml")
	if err := os.WriteFile(path, []byte("domain: one.example.com\nrate_limit_requests_per_sec: 10\nmax_concurrent_tunnels: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadServerConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(path, cfg)
	var notified bool
	m.Subscribe(func(old, next *ServerConfig) {
		if old.Domain != "one.example.com" || next.Domain != "two.example.com" {
			t.Errorf("listener saw old=%s next=%s", old.Domain, next.Domain)
		}
		notified = true
	})
	if err := os.WriteFile(path, []byte("domain: two.example.com\nrate_limit_requests_per_sec: 20\nmax_concurrent_tunnels: 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	next, err := m.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if next.Domain != "two.example.com" || m.Get().RateLimitRequestsPerSec != 20 {
		t.Fatalf("reload not applied: %#v", next)
	}
	if !notified {
		t.Fatal("listener was not notified")
	}
}

func TestManagerReloadPreservesOldOnError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "portald.yaml")
	if err := os.WriteFile(path, []byte("domain: good.example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadServerConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(path, cfg)
	if err := os.WriteFile(path, []byte("domain: \"\"\nrate_limit_requests_per_sec: -5\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Reload(); err == nil {
		t.Fatal("expected reload error for invalid config")
	}
	if got := m.Get().Domain; got != "good.example.com" {
		t.Fatalf("old config lost, domain = %q", got)
	}
}

func TestManagerHandleSignal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "portald.yaml")
	if err := os.WriteFile(path, []byte("domain: sig.example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadServerConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(path, cfg)
	consumed, err := m.HandleSignal(syscall.SIGHUP)
	if err != nil || !consumed {
		t.Fatalf("SIGHUP consumed=%v err=%v", consumed, err)
	}
	if consumed, _ := m.HandleSignal(syscall.SIGTERM); consumed {
		t.Fatal("SIGTERM should not be consumed")
	}
	if _, err := NewManager("", cfg).Reload(); err == nil {
		t.Fatal("expected error when path is empty")
	}
}

func TestManagerWatchCancels(t *testing.T) {
	m := NewManager("", DefaultServerConfig())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.Watch(ctx, nil); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Watch did not exit on cancel")
	}
}

func TestExtractDynamicSettings(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.AuthTokens = []string{"a"}
	dyn := ExtractDynamicSettings(cfg)
	if dyn.RateLimitRequestsPerSec != cfg.RateLimitRequestsPerSec || len(dyn.AuthTokens) != 1 {
		t.Fatalf("dynamic snapshot mismatch: %#v", dyn)
	}
	if len(ExtractDynamicSettings(nil).AuthTokens) != 0 {
		t.Fatal("nil config should yield empty settings")
	}
}
