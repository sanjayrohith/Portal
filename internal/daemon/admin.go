package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/sanjayrohith/portal/internal/config"
	"github.com/sanjayrohith/portal/pkg/auth"
	"github.com/sanjayrohith/portal/pkg/registry"
)

// TunnelInfo describes a single active tunnel for operator listings.
type TunnelInfo struct {
	Subdomain  string    `json:"subdomain"`
	Owner      string    `json:"owner"`
	Active     bool      `json:"active"`
	CreatedAt  time.Time `json:"created_at,omitempty"`
	LastActive time.Time `json:"last_active_at,omitempty"`
}

// GenerateOperatorToken creates a secure token, registers it in store, and
// returns the raw secret (shown once) alongside the stored record.
func GenerateOperatorToken(store *auth.MemoryTokenStore, owner string, maxTunnels, rateLimit int) (string, *auth.Token, error) {
	if store == nil {
		return "", nil, fmt.Errorf("token store cannot be nil")
	}
	owner = strings.TrimSpace(owner)
	if owner == "" {
		owner = "operator"
	}
	if maxTunnels <= 0 {
		maxTunnels = 5
	}
	if rateLimit <= 0 {
		rateLimit = 100
	}
	raw, err := auth.GenerateSecureToken()
	if err != nil {
		return "", nil, err
	}
	record, err := store.RegisterRawToken(raw, owner, maxTunnels, rateLimit)
	if err != nil {
		return "", nil, err
	}
	return raw, record, nil
}

// AppendTokenToConfig persists a raw token into the server config file so it
// survives restarts and SIGHUP reloads. Duplicates (by hash) are ignored.
func AppendTokenToConfig(cfgPath, rawToken string) error {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return fmt.Errorf("token cannot be empty")
	}
	cfg, err := config.LoadServerConfig(cfgPath)
	if err != nil {
		return err
	}
	want := auth.HashToken(rawToken)
	for _, existing := range cfg.AuthTokens {
		if auth.HashToken(existing) == want {
			return nil
		}
	}
	cfg.AuthTokens = append(cfg.AuthTokens, rawToken)
	return config.SaveServerConfig(cfgPath, cfg)
}

// RevokeTokenInConfig removes the token matching selector (raw value, hash,
// ID prefix, or owner) from the config file. It reports whether a token was
// removed so callers can also revoke it in the live in-memory store.
func RevokeTokenInConfig(cfgPath, selector string) (bool, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return false, fmt.Errorf("token selector cannot be empty")
	}
	cfg, err := config.LoadServerConfig(cfgPath)
	if err != nil {
		return false, err
	}
	kept := cfg.AuthTokens[:0:0]
	removed := false
	for _, existing := range cfg.AuthTokens {
		if !removed && tokenMatchesSelector(existing, selector) {
			removed = true
			continue
		}
		kept = append(kept, existing)
	}
	if !removed {
		return false, fmt.Errorf("no token matching %q in %s", selector, cfgPath)
	}
	cfg.AuthTokens = kept
	if err := config.SaveServerConfig(cfgPath, cfg); err != nil {
		return false, err
	}
	return true, nil
}

func tokenMatchesSelector(rawToken, selector string) bool {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == selector {
		return true
	}
	hash := auth.HashToken(rawToken)
	if hash == selector {
		return true
	}
	id := hash[:12]
	if strings.HasPrefix(id, strings.ToLower(selector)) || strings.HasPrefix(hash, strings.ToLower(selector)) {
		return true
	}
	return false
}

// SyncTokensFromConfig registers every config token in store (idempotent) so
// startup and SIGHUP reloads pick up additions without dropping connections.
func SyncTokensFromConfig(store *auth.MemoryTokenStore, cfg *config.ServerConfig) error {
	if store == nil || cfg == nil {
		return fmt.Errorf("store and config cannot be nil")
	}
	for i, raw := range cfg.AuthTokens {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if _, err := store.ValidateToken(raw); err == nil {
			continue
		}
		owner := fmt.Sprintf("config-token-%d", i+1)
		if _, err := store.RegisterRawToken(raw, owner, cfg.MaxConcurrentTunnels, cfg.RateLimitRequestsPerSec); err != nil {
			return fmt.Errorf("register config token %d: %w", i, err)
		}
	}
	return nil
}

// ListActiveTunnels snapshots the registry into a sorted operator listing.
func ListActiveTunnels(reg *registry.SubdomainRegistry) []TunnelInfo {
	if reg == nil {
		return nil
	}
	names := reg.ListActiveSubdomains()
	out := make([]TunnelInfo, 0, len(names))
	for _, name := range names {
		info := TunnelInfo{Subdomain: name, Active: true}
		if record, ok := reg.GetRecord(name); ok {
			info.Owner = record.Owner
			info.CreatedAt = record.CreatedAt
			info.LastActive = record.LastActiveAt
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Subdomain < out[j].Subdomain })
	return out
}

// TunnelsHandler exposes GET /api/tunnels for the tunnels list CLI command.
func TunnelsHandler(reg *registry.SubdomainRegistry) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"tunnels": ListActiveTunnels(reg)})
	})
}
