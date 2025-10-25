package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadClientConfig(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "config.yaml")
	contents := []byte("server_addr: tunnel.example.com:443\nlocal_target: localhost:3000\nsubdomain: demo\nauth_token: secret\nhost_header: rewrite\nkeepalive_interval: 20s\nlog_level: debug\n")
	if err := os.WriteFile(path, contents, 0600); err != nil {
		t.Fatal(err)
	}

	config, err := LoadClientConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.ServerAddr != "tunnel.example.com:443" || config.LocalTarget != "localhost:3000" || config.AuthToken != "secret" || config.KeepAliveInterval != 20*time.Second {
		t.Fatalf("unexpected loaded config: %#v", config)
	}
	if err := config.Validate(); err != nil {
		t.Fatalf("loaded config validation failed: %v", err)
	}
}

func TestLoadClientConfigIfExistsUsesDefaults(t *testing.T) {
	config, err := LoadClientConfigIfExists(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if config.ServerAddr != DefaultClientConfig().ServerAddr {
		t.Fatal("missing config did not use defaults")
	}
}
