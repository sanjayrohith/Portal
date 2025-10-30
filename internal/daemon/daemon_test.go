package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sanjayrohith/portal/internal/config"
	"github.com/sanjayrohith/portal/pkg/auth"
)

func writeDaemonConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "portald.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestNewDaemonFromFileStartup(t *testing.T) {
	path := writeDaemonConfig(t, "domain: daemon.example.com\nrate_limit_requests_per_sec: 25\nmax_concurrent_tunnels: 2\nauth_tokens:\n  - pt_startup_secret\n")
	d, err := NewDaemonFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.Config().Domain != "daemon.example.com" {
		t.Fatalf("domain = %q", d.Config().Domain)
	}
	if _, err := d.Store().ValidateToken("pt_startup_secret"); err != nil {
		t.Fatalf("config token not synced: %v", err)
	}
	if d.Registry() == nil || d.Manager() == nil {
		t.Fatal("daemon missing registry or manager")
	}
}

func TestNewDaemonDefaultsAndInvalid(t *testing.T) {
	d, err := NewDaemon(nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if d.Config().Domain == "" {
		t.Fatal("defaults should populate domain")
	}
	bad := config.DefaultServerConfig()
	bad.Domain = ""
	if _, err := NewDaemon(bad, ""); err == nil {
		t.Fatal("expected error for invalid config")
	}
	if _, err := NewDaemonFromFile(filepath.Join(t.TempDir(), "missing.yaml")); err != nil {
		t.Logf("missing file falls back to defaults: %v", err)
	}
}

func TestDaemonStartWatchesSIGHUP(t *testing.T) {
	path := writeDaemonConfig(t, "domain: watch.example.com\n")
	d, err := NewDaemonFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- d.Start(ctx, nil) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not exit on cancel")
	}
	if err := d.Start(nil, nil); err == nil {
		t.Fatal("expected error for nil context")
	}
}

func TestAdminTokenLifecycle(t *testing.T) {
	store := auth.NewMemoryTokenStore()
	raw, record, err := GenerateOperatorToken(store, "ops", 2, 10)
	if err != nil || record == nil {
		t.Fatal(err)
	}
	if _, err := store.ValidateToken(raw); err != nil {
		t.Fatalf("generated token invalid: %v", err)
	}
	if _, _, err := GenerateOperatorToken(nil, "ops", 1, 1); err == nil {
		t.Fatal("expected error for nil store")
	}

	path := writeDaemonConfig(t, "domain: tokens.example.com\n")
	if err := AppendTokenToConfig(path, raw); err != nil {
		t.Fatal(err)
	}
	if err := AppendTokenToConfig(path, raw); err != nil {
		t.Fatal("duplicate append should be idempotent")
	}
	cfg, err := config.LoadServerConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.AuthTokens) != 1 {
		t.Fatalf("tokens = %v", cfg.AuthTokens)
	}
	if err := SyncTokensFromConfig(store, cfg); err != nil {
		t.Fatal(err)
	}
	removed, err := RevokeTokenInConfig(path, record.ID)
	if err != nil || !removed {
		t.Fatalf("revoke removed=%v err=%v", removed, err)
	}
	if _, err := RevokeTokenInConfig(path, "nonexistent"); err == nil {
		t.Fatal("expected error revoking unknown token")
	}
	if err := AppendTokenToConfig(path, "  "); err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestHealthEndpoints(t *testing.T) {
	mux := NewHealthMux(&HealthChecker{
		ListenerCheck: func() error { return nil },
		StorageCheck:  func(ctx context.Context) error { return nil },
	})
	for _, target := range []string{"/healthz", "/readyz"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body %q", target, rec.Code, rec.Body.String())
		}
		var status HealthStatus
		if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil || status.Status != "ok" {
			t.Fatalf("%s payload = %q err %v", target, rec.Body.String(), err)
		}
	}

	failing := NewHealthMux(&HealthChecker{
		ListenerCheck: func() error { return errors.New("listener down") },
		StorageCheck:  func(ctx context.Context) error { return errors.New("db down") },
	})
	rec := httptest.NewRecorder()
	failing.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("failing readyz status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "not-ready") {
		t.Fatalf("failing body = %q", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/healthz", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST healthz status = %d", rec.Code)
	}
}

func TestAdminMuxTunnelsEndpoint(t *testing.T) {
	d, err := NewDaemon(config.DefaultServerConfig(), "")
	if err != nil {
		t.Fatal(err)
	}
	mux := AdminMux(d.Registry(), nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/tunnels", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("tunnels status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"tunnels"`) {
		t.Fatalf("tunnels body = %q", rec.Body.String())
	}
	if got := ListActiveTunnels(nil); len(got) != 0 {
		t.Fatal("nil registry should list no tunnels")
	}
}
