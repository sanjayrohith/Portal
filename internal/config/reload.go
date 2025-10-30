package config

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// DynamicSettings captures the hot-reloadable subset of ServerConfig that can
// be applied without restarting listeners: token revocation and rate limits.
type DynamicSettings struct {
	AuthTokens              []string
	RateLimitRequestsPerSec int
	MaxConcurrentTunnels    int
	MaxRequestBodySize      int64
	LogLevel                string
}

// ExtractDynamicSettings snapshots the reloadable fields of cfg.
func ExtractDynamicSettings(cfg *ServerConfig) DynamicSettings {
	if cfg == nil {
		return DynamicSettings{}
	}
	return DynamicSettings{
		AuthTokens:              append([]string(nil), cfg.AuthTokens...),
		RateLimitRequestsPerSec: cfg.RateLimitRequestsPerSec,
		MaxConcurrentTunnels:    cfg.MaxConcurrentTunnels,
		MaxRequestBodySize:      cfg.MaxRequestBodySize,
		LogLevel:                cfg.LogLevel,
	}
}

// Manager holds the active daemon configuration and reloads it on demand.
// Listeners are invoked synchronously after a successful reload so the daemon
// can re-apply tokens and rate limits without dropping connections.
type Manager struct {
	mu        sync.RWMutex
	path      string
	cfg       *ServerConfig
	listeners []func(old, next *ServerConfig)
}

// NewManager creates a Manager for path seeded with cfg.
func NewManager(path string, cfg *ServerConfig) *Manager {
	if cfg == nil {
		cfg = DefaultServerConfig()
	}
	return &Manager{path: path, cfg: cfg}
}

// Path returns the watched configuration file path.
func (m *Manager) Path() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.path
}

// Get returns a deep copy of the active configuration.
func (m *Manager) Get() *ServerConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cp := *m.cfg
	cp.AuthTokens = append([]string(nil), m.cfg.AuthTokens...)
	return &cp
}

// Subscribe registers a callback invoked after every successful reload.
func (m *Manager) Subscribe(fn func(old, next *ServerConfig)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, fn)
}

// Reload re-reads the configuration file, validates it, swaps it in, and
// notifies listeners. The previous configuration is preserved on error.
func (m *Manager) Reload() (*ServerConfig, error) {
	m.mu.RLock()
	path := m.path
	m.mu.RUnlock()

	if path == "" {
		return nil, fmt.Errorf("config reload failed: no config file path set")
	}
	next, err := LoadServerConfig(path)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	old := m.cfg
	m.cfg = next
	listeners := append([]func(old, next *ServerConfig){}, m.listeners...)
	m.mu.Unlock()

	for _, fn := range listeners {
		fn(old, next)
	}
	return next, nil
}

// HandleSignal reloads on SIGHUP and reports whether the signal was consumed.
func (m *Manager) HandleSignal(sig os.Signal) (bool, error) {
	if sig == syscall.SIGHUP {
		_, err := m.Reload()
		return true, err
	}
	return false, nil
}

// Watch blocks until ctx ends, reloading the configuration on every SIGHUP.
// Errors are forwarded to onError without terminating the watch.
func (m *Manager) Watch(ctx context.Context, onError func(error)) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGHUP)
	defer signal.Stop(signals)

	for {
		select {
		case <-ctx.Done():
			return
		case sig := <-signals:
			if _, err := m.HandleSignal(sig); err != nil && onError != nil {
				onError(err)
			}
		}
	}
}
