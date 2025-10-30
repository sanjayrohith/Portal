package daemon

import (
	"context"
	"fmt"

	"github.com/sanjayrohith/portal/internal/config"
	"github.com/sanjayrohith/portal/pkg/auth"
	"github.com/sanjayrohith/portal/pkg/registry"
)

// Daemon wires the control plane lifecycle: configuration, token store, and
// subdomain registry. Listener serving is composed on top via Start.
type Daemon struct {
	manager  *config.Manager
	store    *auth.MemoryTokenStore
	registry *registry.SubdomainRegistry
}

// NewDaemon builds a daemon seeded from cfg, remembering cfgPath for SIGHUP
// reloads. Config tokens are synced into a fresh in-memory store.
func NewDaemon(cfg *config.ServerConfig, cfgPath string) (*Daemon, error) {
	if cfg == nil {
		cfg = config.DefaultServerConfig()
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid daemon config: %w", err)
	}
	store := auth.NewMemoryTokenStore()
	if err := SyncTokensFromConfig(store, cfg); err != nil {
		return nil, err
	}
	manager := config.NewManager(cfgPath, cfg)
	d := &Daemon{manager: manager, store: store, registry: registry.NewSubdomainRegistry()}
	manager.Subscribe(func(_, next *config.ServerConfig) {
		_ = SyncTokensFromConfig(d.store, next)
	})
	return d, nil
}

// NewDaemonFromFile loads cfgPath (or defaults when absent) and builds a Daemon.
func NewDaemonFromFile(cfgPath string) (*Daemon, error) {
	cfg, err := config.LoadServerConfigIfExists(cfgPath)
	if err != nil {
		return nil, err
	}
	return NewDaemon(cfg, cfgPath)
}

// Config returns a snapshot of the active configuration.
func (d *Daemon) Config() *config.ServerConfig { return d.manager.Get() }

// Store exposes the authentication token store.
func (d *Daemon) Store() *auth.MemoryTokenStore { return d.store }

// Registry exposes the subdomain routing registry.
func (d *Daemon) Registry() *registry.SubdomainRegistry { return d.registry }

// Manager exposes the SIGHUP-reloadable configuration manager.
func (d *Daemon) Manager() *config.Manager { return d.manager }

// Start begins background SIGHUP watching; it blocks until ctx is cancelled.
func (d *Daemon) Start(ctx context.Context, onError func(error)) error {
	if ctx == nil {
		return fmt.Errorf("context cannot be nil")
	}
	d.manager.Watch(ctx, onError)
	return nil
}
