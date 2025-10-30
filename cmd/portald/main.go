package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/sanjayrohith/portal/internal/config"
	"github.com/sanjayrohith/portal/internal/daemon"
	"github.com/sanjayrohith/portal/pkg/auth"
	"github.com/sanjayrohith/portal/pkg/logger"
)

var (
	serverCfg = config.DefaultServerConfig()
	cfgFile   string

	tokenOwner      string
	tokenMaxTunnels int
	tokenRateLimit  int
	tunnelsAdmin    string

	rootCmd = &cobra.Command{
		Use:   "portald",
		Short: "portald is the control plane and edge proxy server for Portal tunnels",
		Long: `portald accepts inbound TLS connections from portal clients, manages subdomain
routing registrations, and proxies incoming public HTTP/HTTPS traffic to clients.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, resolvedPath, err := resolveServerConfig(cmd)
			if err != nil {
				return err
			}
			if err := cfg.Validate(); err != nil {
				return fmt.Errorf("configuration validation failed: %w", err)
			}

			log := logger.New(logger.Options{
				Level:  cfg.LogLevel,
				Format: "text",
			})

			d, err := daemon.NewDaemon(cfg, resolvedPath)
			if err != nil {
				return err
			}

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			go d.Manager().Watch(ctx, func(err error) {
				log.Error("config reload failed", "error", err)
			})

			log.Info("starting portal daemon (portald)",
				"domain", cfg.Domain,
				"control_addr", cfg.ControlAddr,
				"http_addr", cfg.HTTPAddr,
				"https_addr", cfg.HTTPSAddr,
				"storage_type", cfg.StorageType,
				"config", resolvedPath,
			)
			<-ctx.Done()
			log.Info("shutting down portal daemon")
			return nil
		},
	}

	tokensCmd = &cobra.Command{
		Use:   "tokens",
		Short: "Manage authentication tokens for tunnel clients",
	}

	tokensGenerateCmd = &cobra.Command{
		Use:   "generate",
		Short: "Generate a new authentication token",
		RunE: func(cmd *cobra.Command, args []string) error {
			store := auth.NewMemoryTokenStore()
			raw, record, err := daemon.GenerateOperatorToken(store, tokenOwner, tokenMaxTunnels, tokenRateLimit)
			if err != nil {
				return err
			}
			path := effectiveConfigPath()
			if path != "" {
				if _, statErr := os.Stat(path); statErr == nil {
					if err := daemon.AppendTokenToConfig(path, raw); err != nil {
						return fmt.Errorf("token generated but config persist failed: %w", err)
					}
					fmt.Fprintf(cmd.OutOrStdout(), "token persisted to %s\n", path)
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "token: %s\nid: %s\nowner: %s\n", raw, record.ID, record.Owner)
			return nil
		},
	}

	tokensRevokeCmd = &cobra.Command{
		Use:   "revoke <token|id|hash-prefix>",
		Short: "Revoke an authentication token (removes it from portald.yaml)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := effectiveConfigPath()
			if path == "" {
				return fmt.Errorf("revocation requires --config pointing at portald.yaml")
			}
			removed, err := daemon.RevokeTokenInConfig(path, args[0])
			if err != nil {
				return err
			}
			if !removed {
				return fmt.Errorf("no matching token found")
			}
			fmt.Fprintf(cmd.OutOrStdout(), "revoked token matching %q (send SIGHUP to portald to apply without restart)\n", args[0])
			return nil
		},
	}

	tokensListCmd = &cobra.Command{
		Use:   "list",
		Short: "List configured tokens (IDs and hashes, secrets redacted)",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := effectiveConfigPath()
			if path == "" {
				return fmt.Errorf("listing requires --config pointing at portald.yaml")
			}
			cfg, err := config.LoadServerConfig(path)
			if err != nil {
				return err
			}
			if len(cfg.AuthTokens) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no tokens configured")
				return nil
			}
			for _, raw := range cfg.AuthTokens {
				hash := auth.HashToken(raw)
				fmt.Fprintf(cmd.OutOrStdout(), "id: %s hash: %.16s... owner: config\n", hash[:12], hash)
			}
			return nil
		},
	}

	tunnelsCmd = &cobra.Command{
		Use:   "tunnels",
		Short: "Inspect active tunnels on a running daemon",
	}

	tunnelsListCmd = &cobra.Command{
		Use:   "list",
		Short: "List active tunnels via the admin API",
		RunE: func(cmd *cobra.Command, args []string) error {
			addr := strings.TrimSpace(tunnelsAdmin)
			if addr == "" {
				addr = serverCfg.AdminAddr
			}
			url := "http://" + strings.TrimPrefix(addr, "http://") + "/api/tunnels"
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Get(url)
			if err != nil {
				return fmt.Errorf("query admin API %s: %w (is portald running?)", url, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("admin API returned %s", resp.Status)
			}
			var payload struct {
				Tunnels []daemon.TunnelInfo `json:"tunnels"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
				return fmt.Errorf("decode tunnels response: %w", err)
			}
			if len(payload.Tunnels) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no active tunnels")
				return nil
			}
			for _, t := range payload.Tunnels {
				fmt.Fprintf(cmd.OutOrStdout(), "%s owner=%s active=%v\n", t.Subdomain, t.Owner, t.Active)
			}
			return nil
		},
	}
)

func effectiveConfigPath() string {
	if cfgFile != "" {
		return cfgFile
	}
	return config.DefaultServerConfigPath()
}

func resolveServerConfig(cmd *cobra.Command) (*config.ServerConfig, string, error) {
	path := ""
	if cfgFile != "" {
		path = cfgFile
		cfg, err := config.LoadServerConfig(path)
		if err != nil {
			return nil, path, err
		}
		mergeFlagOverrides(cmd, cfg)
		return cfg, path, nil
	}
	defaults := config.DefaultServerConfig()
	candidate := config.DefaultServerConfigPath()
	if _, err := os.Stat(candidate); err == nil {
		cfg, err := config.LoadServerConfig(candidate)
		if err != nil {
			return nil, candidate, err
		}
		path = candidate
		mergeFlagOverrides(cmd, cfg)
		return cfg, path, nil
	}
	mergeFlagOverrides(cmd, defaults)
	return defaults, "", nil
}

func mergeFlagOverrides(cmd *cobra.Command, cfg *config.ServerConfig) {
	// Flag-bound serverCfg values override file values only when the user
	// explicitly changed them on the command line.
	if cmd.PersistentFlags().Changed("domain") {
		cfg.Domain = serverCfg.Domain
	}
	if cmd.PersistentFlags().Changed("control-addr") {
		cfg.ControlAddr = serverCfg.ControlAddr
	}
	if cmd.PersistentFlags().Changed("http-addr") {
		cfg.HTTPAddr = serverCfg.HTTPAddr
	}
	if cmd.PersistentFlags().Changed("https-addr") {
		cfg.HTTPSAddr = serverCfg.HTTPSAddr
	}
	if cmd.PersistentFlags().Changed("admin-addr") {
		cfg.AdminAddr = serverCfg.AdminAddr
	}
	if cmd.PersistentFlags().Changed("storage-type") {
		cfg.StorageType = serverCfg.StorageType
	}
	if cmd.PersistentFlags().Changed("storage-dsn") {
		cfg.StorageDSN = serverCfg.StorageDSN
	}
	if cmd.PersistentFlags().Changed("rate-limit") {
		cfg.RateLimitRequestsPerSec = serverCfg.RateLimitRequestsPerSec
	}
	if cmd.PersistentFlags().Changed("max-tunnels") {
		cfg.MaxConcurrentTunnels = serverCfg.MaxConcurrentTunnels
	}
	if cmd.PersistentFlags().Changed("log-level") {
		cfg.LogLevel = serverCfg.LogLevel
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "Path to server configuration YAML file")
	rootCmd.PersistentFlags().StringVarP(&serverCfg.Domain, "domain", "d", serverCfg.Domain, "Base domain for tunnels (e.g. yourdomain.com)")
	rootCmd.PersistentFlags().StringVar(&serverCfg.ControlAddr, "control-addr", serverCfg.ControlAddr, "Address for incoming client control connections")
	rootCmd.PersistentFlags().StringVar(&serverCfg.HTTPAddr, "http-addr", serverCfg.HTTPAddr, "Address for public HTTP traffic")
	rootCmd.PersistentFlags().StringVar(&serverCfg.HTTPSAddr, "https-addr", serverCfg.HTTPSAddr, "Address for public HTTPS traffic")
	rootCmd.PersistentFlags().StringVar(&serverCfg.AdminAddr, "admin-addr", serverCfg.AdminAddr, "Address for admin and metrics endpoint")
	rootCmd.PersistentFlags().StringVar(&serverCfg.StorageType, "storage-type", serverCfg.StorageType, "Persistence storage type (sqlite or postgres)")
	rootCmd.PersistentFlags().StringVar(&serverCfg.StorageDSN, "storage-dsn", serverCfg.StorageDSN, "Storage data source name / database path")
	rootCmd.PersistentFlags().IntVar(&serverCfg.RateLimitRequestsPerSec, "rate-limit", serverCfg.RateLimitRequestsPerSec, "Maximum requests per second per token")
	rootCmd.PersistentFlags().IntVar(&serverCfg.MaxConcurrentTunnels, "max-tunnels", serverCfg.MaxConcurrentTunnels, "Maximum concurrent tunnels per token")
	rootCmd.PersistentFlags().StringVar(&serverCfg.LogLevel, "log-level", serverCfg.LogLevel, "Logging level (debug, info, warn, error)")

	tokensGenerateCmd.Flags().StringVar(&tokenOwner, "owner", "operator", "Owner label recorded for the new token")
	tokensGenerateCmd.Flags().IntVar(&tokenMaxTunnels, "max-tunnels", 5, "Maximum concurrent tunnels for the new token")
	tokensGenerateCmd.Flags().IntVar(&tokenRateLimit, "rate-limit", 100, "Requests per second allowed for the new token")
	tunnelsListCmd.Flags().StringVar(&tunnelsAdmin, "admin-addr", "", "Admin API address (default from config)")

	tokensCmd.AddCommand(tokensGenerateCmd)
	tokensCmd.AddCommand(tokensRevokeCmd)
	tokensCmd.AddCommand(tokensListCmd)
	tunnelsCmd.AddCommand(tunnelsListCmd)
	rootCmd.AddCommand(tokensCmd)
	rootCmd.AddCommand(tunnelsCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
