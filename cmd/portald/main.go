package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sanjayrohith/portal/internal/config"
	"github.com/sanjayrohith/portal/pkg/logger"
)

var (
	serverCfg = config.DefaultServerConfig()
	cfgFile   string

	rootCmd = &cobra.Command{
		Use:   "portald",
		Short: "portald is the control plane and edge proxy server for Portal tunnels",
		Long: `portald accepts inbound TLS connections from portal clients, manages subdomain
routing registrations, and proxies incoming public HTTP/HTTPS traffic to clients.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := serverCfg.Validate(); err != nil {
				return fmt.Errorf("configuration validation failed: %w", err)
			}

			log := logger.New(logger.Options{
				Level:  serverCfg.LogLevel,
				Format: "text",
			})

			log.Info("starting portal daemon (portald)",
				"domain", serverCfg.Domain,
				"control_addr", serverCfg.ControlAddr,
				"http_addr", serverCfg.HTTPAddr,
				"https_addr", serverCfg.HTTPSAddr,
				"storage_type", serverCfg.StorageType,
			)
			return nil
		},
	}
)

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
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
