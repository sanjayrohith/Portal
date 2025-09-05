package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sanjayrohith/portal/internal/config"
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

			fmt.Printf("Starting Portal Daemon (portald)\n")
			fmt.Printf("  Domain:        %s\n", serverCfg.Domain)
			fmt.Printf("  Control Addr:  %s\n", serverCfg.ControlAddr)
			fmt.Printf("  HTTP Addr:     %s\n", serverCfg.HTTPAddr)
			fmt.Printf("  HTTPS Addr:    %s\n", serverCfg.HTTPSAddr)
			fmt.Printf("  Admin Addr:    %s\n", serverCfg.AdminAddr)
			fmt.Printf("  Storage:       %s (%s)\n", serverCfg.StorageType, serverCfg.StorageDSN)
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
