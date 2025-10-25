package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// ClientConfig holds configuration parameters for the portal client CLI.
type ClientConfig struct {
	ServerAddr         string        `json:"server_addr" yaml:"server_addr"`
	LocalTarget        string        `json:"local_target" yaml:"local_target"`
	Subdomain          string        `json:"subdomain" yaml:"subdomain"`
	AuthToken          string        `json:"auth_token" yaml:"auth_token"`
	HostHeader         string        `json:"host_header" yaml:"host_header"`
	InspectorAddr      string        `json:"inspector_addr" yaml:"inspector_addr"`
	InsecureSkipVerify bool          `json:"insecure_skip_verify" yaml:"insecure_skip_verify"`
	KeepAliveInterval  time.Duration `json:"keepalive_interval" yaml:"keepalive_interval"`
	LogLevel           string        `json:"log_level" yaml:"log_level"`
}

// DefaultClientConfig returns a ClientConfig with production-grade defaults.
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		ServerAddr:         "127.0.0.1:8443",
		LocalTarget:        "127.0.0.1:3000",
		Subdomain:          "",
		AuthToken:          "",
		HostHeader:         "",
		InspectorAddr:      "127.0.0.1:4040",
		InsecureSkipVerify: false,
		KeepAliveInterval:  15 * time.Second,
		LogLevel:           "info",
	}
}

// Validate validates the client configuration.
func (c *ClientConfig) Validate() error {
	if c.ServerAddr == "" {
		return fmt.Errorf("server address cannot be empty")
	}
	if strings.Contains(c.ServerAddr, "://") {
		return fmt.Errorf("server address %q includes a URL scheme; use host:port such as tunnel.example.com:443", c.ServerAddr)
	}
	if err := validateHostPort(c.ServerAddr, "server address"); err != nil {
		return err
	}
	if c.LocalTarget == "" {
		return fmt.Errorf("local target cannot be empty")
	}
	if c.Subdomain != "" {
		if err := ValidateSubdomain(c.Subdomain); err != nil {
			return err
		}
	}
	if c.HostHeader != "" && c.HostHeader != "rewrite" {
		return fmt.Errorf("invalid --host-header %q; use empty or 'rewrite'", c.HostHeader)
	}
	if c.InspectorAddr != "" {
		host, _, err := net.SplitHostPort(c.InspectorAddr)
		if err != nil {
			return fmt.Errorf("invalid inspector address %q: %w", c.InspectorAddr, err)
		}
		if host != "127.0.0.1" && host != "localhost" {
			return fmt.Errorf("inspector address must bind strictly to loopback (127.0.0.1), got %q", host)
		}
	}
	return nil
}

// NormalizeLocalTarget normalizes input like "3000", ":3000", or "localhost:3000" into "127.0.0.1:3000".
func NormalizeLocalTarget(target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", fmt.Errorf("empty local target")
	}
	if strings.Contains(target, "://") {
		return "", fmt.Errorf("local target %q includes a URL scheme; use host:port or port only", target)
	}
	if port, err := strconv.Atoi(target); err == nil {
		if port <= 0 || port > 65535 {
			return "", fmt.Errorf("invalid port number %d", port)
		}
		return fmt.Sprintf("127.0.0.1:%d", port), nil
	}
	if strings.HasPrefix(target, ":") {
		port, err := strconv.Atoi(target[1:])
		if err != nil || port <= 0 || port > 65535 {
			return "", fmt.Errorf("invalid target port specification %q", target)
		}
		return fmt.Sprintf("127.0.0.1:%d", port), nil
	}
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return "", fmt.Errorf("invalid target format %q, expected host:port or port: %w", target, err)
	}
	if host == "localhost" {
		host = "127.0.0.1"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return "", fmt.Errorf("invalid target port %q", portStr)
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

func validateHostPort(address, label string) error {
	host, portString, err := net.SplitHostPort(address)
	if err != nil || host == "" {
		return fmt.Errorf("invalid %s %q; use host:port such as 127.0.0.1:8443", label, address)
	}
	port, err := strconv.Atoi(portString)
	if err != nil || port <= 0 || port > 65535 {
		return fmt.Errorf("invalid %s port %q; choose a number from 1 to 65535", label, portString)
	}
	return nil
}

// ValidateSubdomain validates a single RFC 1123 DNS label for tunnel routing.
func ValidateSubdomain(subdomain string) error {
	if subdomain == "" {
		return fmt.Errorf("invalid subdomain; provide a non-empty DNS label")
	}
	if len(subdomain) > 63 {
		return fmt.Errorf("invalid subdomain %q; use 1-63 lowercase letters, digits, or hyphens", subdomain)
	}
	if subdomain != strings.ToLower(subdomain) || subdomain[0] == '-' || subdomain[len(subdomain)-1] == '-' {
		return fmt.Errorf("invalid subdomain %q; use lowercase and do not begin or end with a hyphen", subdomain)
	}
	for _, character := range subdomain {
		if character != '-' && (character < 'a' || character > 'z') && (character < '0' || character > '9') {
			return fmt.Errorf("invalid subdomain %q; use lowercase letters, digits, or hyphens", subdomain)
		}
	}
	return nil
}

// ServerConfig holds configuration parameters for the portald server daemon.
type ServerConfig struct {
	Domain                  string        `json:"domain" yaml:"domain"`
	ControlAddr             string        `json:"control_addr" yaml:"control_addr"`
	HTTPAddr                string        `json:"http_addr" yaml:"http_addr"`
	HTTPSAddr               string        `json:"https_addr" yaml:"https_addr"`
	AdminAddr               string        `json:"admin_addr" yaml:"admin_addr"`
	StorageType             string        `json:"storage_type" yaml:"storage_type"`
	StorageDSN              string        `json:"storage_dsn" yaml:"storage_dsn"`
	CertFile                string        `json:"cert_file" yaml:"cert_file"`
	KeyFile                 string        `json:"key_file" yaml:"key_file"`
	ACMEEmail               string        `json:"acme_email" yaml:"acme_email"`
	ACMEDir                 string        `json:"acme_dir" yaml:"acme_dir"`
	RateLimitRequestsPerSec int           `json:"rate_limit_requests_per_sec" yaml:"rate_limit_requests_per_sec"`
	MaxConcurrentTunnels    int           `json:"max_concurrent_tunnels" yaml:"max_concurrent_tunnels"`
	MaxRequestBodySize      int64         `json:"max_request_body_size" yaml:"max_request_body_size"`
	IdleTimeout             time.Duration `json:"idle_timeout" yaml:"idle_timeout"`
	LogLevel                string        `json:"log_level" yaml:"log_level"`
}

// DefaultServerConfig returns a ServerConfig with production-grade defaults.
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Domain:                  "localhost",
		ControlAddr:             ":8443",
		HTTPAddr:                ":8080",
		HTTPSAddr:               ":8444",
		AdminAddr:               "127.0.0.1:9090",
		StorageType:             "sqlite",
		StorageDSN:              "portal.db",
		CertFile:                "",
		KeyFile:                 "",
		ACMEEmail:               "",
		ACMEDir:                 "certs",
		RateLimitRequestsPerSec: 100,
		MaxConcurrentTunnels:    5,
		MaxRequestBodySize:      10 * 1024 * 1024, // 10MB
		IdleTimeout:             60 * time.Second,
		LogLevel:                "info",
	}
}

// Validate validates the server configuration.
func (s *ServerConfig) Validate() error {
	if s.Domain == "" {
		return fmt.Errorf("domain cannot be empty")
	}
	if s.ControlAddr == "" {
		return fmt.Errorf("control address cannot be empty")
	}
	if s.RateLimitRequestsPerSec <= 0 {
		return fmt.Errorf("rate limit must be positive, got %d", s.RateLimitRequestsPerSec)
	}
	if s.MaxConcurrentTunnels <= 0 {
		return fmt.Errorf("max concurrent tunnels must be positive, got %d", s.MaxConcurrentTunnels)
	}
	return nil
}
