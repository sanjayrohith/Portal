package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sanjayrohith/portal/internal/config"
)

var (
	clientCfg = config.DefaultClientConfig()

	rootCmd = &cobra.Command{
		Use:   "portal",
		Short: "Portal is an open-source localhost tunneling client",
		Long: `Portal punches secure, persistent tunnels through NAT and corporate firewalls
to expose local HTTP servers on stable, custom subdomains.`,
	}

	httpCmd = &cobra.Command{
		Use:   "http <port or address>",
		Short: "Expose a local HTTP server publicly",
		Args:  cobra.ExactArgs(1),
		Example: `  portal http 3000 --subdomain myapp
  portal http 3000 --subdomain myapp --host-header rewrite
  portal http localhost:8080 --server tunnel.example.com:443`,
		RunE: func(cmd *cobra.Command, args []string) error {
			normalized, err := config.NormalizeLocalTarget(args[0])
			if err != nil {
				return err
			}
			clientCfg.LocalTarget = normalized

			if err := clientCfg.Validate(); err != nil {
				return fmt.Errorf("configuration validation failed: %w", err)
			}

			fmt.Printf("Starting tunnel for target: %s\n", clientCfg.LocalTarget)
			fmt.Printf("Requested subdomain: %s\n", clientCfg.Subdomain)
			fmt.Printf("Control plane server: %s\n", clientCfg.ServerAddr)
			fmt.Printf("Inspector UI binding: %s\n", clientCfg.InspectorAddr)
			return nil
		},
	}

	statusCmd = &cobra.Command{
		Use:   "status",
		Short: "Display active tunnel status and session statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("No active portal sessions found.")
			return nil
		},
	}
)

func init() {
	httpCmd.Flags().StringVarP(&clientCfg.Subdomain, "subdomain", "s", clientCfg.Subdomain, "Custom subdomain to request from the server")
	httpCmd.Flags().StringVar(&clientCfg.ServerAddr, "server", clientCfg.ServerAddr, "Control plane server address (host:port)")
	httpCmd.Flags().StringVarP(&clientCfg.AuthToken, "token", "t", clientCfg.AuthToken, "Authentication token for tunnel allocation")
	httpCmd.Flags().StringVar(&clientCfg.HostHeader, "host-header", clientCfg.HostHeader, "Rewrite Host header (e.g. 'rewrite' to match localhost)")
	httpCmd.Flags().StringVar(&clientCfg.InspectorAddr, "inspector-addr", clientCfg.InspectorAddr, "Address for local request inspector UI (must bind to 127.0.0.1)")
	httpCmd.Flags().BoolVar(&clientCfg.InsecureSkipVerify, "insecure", clientCfg.InsecureSkipVerify, "Skip TLS certificate verification (development only)")
	httpCmd.Flags().StringVar(&clientCfg.LogLevel, "log-level", clientCfg.LogLevel, "Logging level (debug, info, warn, error)")

	rootCmd.AddCommand(httpCmd)
	rootCmd.AddCommand(statusCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
