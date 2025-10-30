package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultServerConfigPath returns the conventional portald.yaml location.
func DefaultServerConfigPath() string {
	if _, err := os.Stat("portald.yaml"); err == nil {
		return "portald.yaml"
	}
	return "/etc/portald/portald.yaml"
}

// LoadServerConfig loads a YAML or JSON server configuration file over
// production defaults. An empty path resolves via DefaultServerConfigPath.
// JSON documents are accepted because JSON is a subset of YAML; files ending
// in .json are additionally validated with encoding/json for strict syntax.
func LoadServerConfig(path string) (*ServerConfig, error) {
	if path == "" {
		path = DefaultServerConfigPath()
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read server config %q: %w", path, err)
	}
	if strings.HasSuffix(strings.ToLower(path), ".json") {
		if !json.Valid(contents) {
			return nil, fmt.Errorf("parse server config %q: invalid JSON document", path)
		}
	}
	var file serverConfigFile
	if err := yaml.Unmarshal(contents, &file); err != nil {
		return nil, fmt.Errorf("parse server config %q: %w", path, err)
	}

	cfg := DefaultServerConfig()
	file.applyTo(cfg)
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid server config %q: %w", path, err)
	}
	return cfg, nil
}

// LoadServerConfigIfExists returns defaults when the file is absent so the
// daemon can boot with flags alone; other errors are returned.
func LoadServerConfigIfExists(path string) (*ServerConfig, error) {
	if path == "" {
		path = DefaultServerConfigPath()
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return DefaultServerConfig(), nil
	} else if err != nil {
		return nil, fmt.Errorf("inspect server config %q: %w", path, err)
	}
	return LoadServerConfig(path)
}

// SaveServerConfig persists cfg as YAML with restrictive permissions.
func SaveServerConfig(path string, cfg *ServerConfig) error {
	if path == "" {
		return fmt.Errorf("config path cannot be empty")
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("refusing to save invalid config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode server config: %w", err)
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		return fmt.Errorf("write server config %q: %w", path, err)
	}
	return nil
}

type serverConfigFile struct {
	Domain                  string         `yaml:"domain" json:"domain"`
	ControlAddr             string         `yaml:"control_addr" json:"control_addr"`
	HTTPAddr                string         `yaml:"http_addr" json:"http_addr"`
	HTTPSAddr               string         `yaml:"https_addr" json:"https_addr"`
	AdminAddr               string         `yaml:"admin_addr" json:"admin_addr"`
	StorageType             string         `yaml:"storage_type" json:"storage_type"`
	StorageDSN              string         `yaml:"storage_dsn" json:"storage_dsn"`
	CertFile                string         `yaml:"cert_file" json:"cert_file"`
	KeyFile                 string         `yaml:"key_file" json:"key_file"`
	ACMEEmail               string         `yaml:"acme_email" json:"acme_email"`
	ACMEDir                 string         `yaml:"acme_dir" json:"acme_dir"`
	AuthTokens              []string       `yaml:"auth_tokens" json:"auth_tokens"`
	MetricsToken            string         `yaml:"metrics_token" json:"metrics_token"`
	RateLimitRequestsPerSec *int           `yaml:"rate_limit_requests_per_sec" json:"rate_limit_requests_per_sec"`
	MaxConcurrentTunnels    *int           `yaml:"max_concurrent_tunnels" json:"max_concurrent_tunnels"`
	MaxRequestBodySize      *int64         `yaml:"max_request_body_size" json:"max_request_body_size"`
	IdleTimeout             *durationValue `yaml:"idle_timeout" json:"idle_timeout"`
	LogLevel                string         `yaml:"log_level" json:"log_level"`
}

func (f *serverConfigFile) applyTo(cfg *ServerConfig) {
	if f.Domain != "" {
		cfg.Domain = f.Domain
	}
	if f.ControlAddr != "" {
		cfg.ControlAddr = f.ControlAddr
	}
	if f.HTTPAddr != "" {
		cfg.HTTPAddr = f.HTTPAddr
	}
	if f.HTTPSAddr != "" {
		cfg.HTTPSAddr = f.HTTPSAddr
	}
	if f.AdminAddr != "" {
		cfg.AdminAddr = f.AdminAddr
	}
	if f.StorageType != "" {
		cfg.StorageType = f.StorageType
	}
	if f.StorageDSN != "" {
		cfg.StorageDSN = f.StorageDSN
	}
	if f.CertFile != "" {
		cfg.CertFile = f.CertFile
	}
	if f.KeyFile != "" {
		cfg.KeyFile = f.KeyFile
	}
	if f.ACMEEmail != "" {
		cfg.ACMEEmail = f.ACMEEmail
	}
	if f.ACMEDir != "" {
		cfg.ACMEDir = f.ACMEDir
	}
	if f.AuthTokens != nil {
		cfg.AuthTokens = append([]string(nil), f.AuthTokens...)
	}
	if f.MetricsToken != "" {
		cfg.MetricsToken = f.MetricsToken
	}
	if f.RateLimitRequestsPerSec != nil {
		cfg.RateLimitRequestsPerSec = *f.RateLimitRequestsPerSec
	}
	if f.MaxConcurrentTunnels != nil {
		cfg.MaxConcurrentTunnels = *f.MaxConcurrentTunnels
	}
	if f.MaxRequestBodySize != nil {
		cfg.MaxRequestBodySize = *f.MaxRequestBodySize
	}
	if f.IdleTimeout != nil {
		cfg.IdleTimeout = f.IdleTimeout.Duration
	}
	if f.LogLevel != "" {
		cfg.LogLevel = f.LogLevel
	}
}

// UnmarshalJSON accepts "60s" strings or nanosecond numbers.
func (d *durationValue) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		value, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", s, err)
		}
		d.Duration = value
		return nil
	}
	var nanoseconds int64
	if err := json.Unmarshal(data, &nanoseconds); err != nil {
		return fmt.Errorf("duration must be a string such as 60s: %w", err)
	}
	d.Duration = time.Duration(nanoseconds)
	return nil
}
