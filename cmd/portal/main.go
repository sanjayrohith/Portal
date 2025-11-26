package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/sanjayrohith/portal/internal/config"
	"github.com/sanjayrohith/portal/pkg/doctor"
	"github.com/sanjayrohith/portal/pkg/logger"
	"github.com/sanjayrohith/portal/pkg/telemetry"
)

var (
	clientCfg     = config.DefaultClientConfig()
	statusAddr    string
	statusToken   string
	statusJSON    bool
	configLoadErr error

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
			if configLoadErr != nil {
				return configLoadErr
			}
			normalized, err := config.NormalizeLocalTarget(args[0])
			if err != nil {
				return err
			}
			clientCfg.LocalTarget = normalized

			if err := clientCfg.Validate(); err != nil {
				return fmt.Errorf("configuration validation failed: %w", err)
			}

			log := logger.New(logger.Options{
				Level:  clientCfg.LogLevel,
				Format: "text",
			})

			log.Info("initializing portal tunnel client",
				"target", clientCfg.LocalTarget,
				"subdomain", clientCfg.Subdomain,
				"server", clientCfg.ServerAddr,
				"inspector", clientCfg.InspectorAddr,
			)
			return nil
		},
	}

	statusCmd = &cobra.Command{
		Use:   "status",
		Short: "Display active tunnel status and session statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			snapshot, err := telemetry.QueryLocalStatus(statusAddr, statusToken)
			if err != nil {
				return err
			}
			output, err := telemetry.FormatStatus(snapshot, statusJSON)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), output)
			return nil
		},
	}

	completionCmd = &cobra.Command{
		Use:   "completion <bash|zsh|fish>",
		Short: "Generate shell completion scripts",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return rootCmd.GenBashCompletion(cmd.OutOrStdout())
			case "zsh":
				return rootCmd.GenZshCompletion(cmd.OutOrStdout())
			case "fish":
				return rootCmd.GenFishCompletion(cmd.OutOrStdout(), true)
			default:
				return fmt.Errorf("unsupported shell %q; choose bash, zsh, or fish", args[0])
			}
		},
	}
	doctorServer    string
	doctorTarget    string
	doctorInspector string
	doctorInsecure  bool
	doctorJSON      bool
	doctorTimeout   time.Duration

	doctorCmd = &cobra.Command{
		Use:   "doctor",
		Short: "Verify local network environment, DNS resolution, and TLS handshake",
		Long: `Doctor performs a battery of diagnostic probes verifying that the local loopback
interface is active, the inspector port is bindable, the control plane hostname resolves
via DNS, TCP connectivity succeeds, and a TLS 1.3 handshake with ALPN negotiation completes.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := doctor.Options{
				ServerAddr:         doctorServer,
				LocalTarget:        doctorTarget,
				InspectorAddr:      doctorInspector,
				InsecureSkipVerify: doctorInsecure,
				Timeout:            doctorTimeout,
			}

			report := doctor.RunDiagnostics(opts)
			if doctorJSON {
				out, err := doctor.FormatJSON(report)
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), out)
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), doctor.FormatText(report))
			}

			if !report.AllPassed {
				return fmt.Errorf("one or more diagnostic checks failed")
			}
			return nil
		},
	}
)

func init() {
	if loaded, err := config.LoadClientConfigIfExists(""); err != nil {
		configLoadErr = err
	} else {
		clientCfg = loaded
	}
	httpCmd.Flags().StringVarP(&clientCfg.Subdomain, "subdomain", "s", clientCfg.Subdomain, "Custom subdomain to request from the server")
	httpCmd.Flags().StringVar(&clientCfg.ServerAddr, "server", clientCfg.ServerAddr, "Control plane server address (host:port)")
	httpCmd.Flags().StringVarP(&clientCfg.AuthToken, "token", "t", clientCfg.AuthToken, "Authentication token for tunnel allocation")
	httpCmd.Flags().StringVar(&clientCfg.HostHeader, "host-header", clientCfg.HostHeader, "Rewrite Host header (e.g. 'rewrite' to match localhost)")
	httpCmd.Flags().StringVar(&clientCfg.InspectorAddr, "inspector-addr", clientCfg.InspectorAddr, "Address for local request inspector UI (must bind to 127.0.0.1)")
	httpCmd.Flags().BoolVar(&clientCfg.InsecureSkipVerify, "insecure", clientCfg.InsecureSkipVerify, "Skip TLS certificate verification (development only)")
	httpCmd.Flags().StringVar(&clientCfg.LogLevel, "log-level", clientCfg.LogLevel, "Logging level (debug, info, warn, error)")

	doctorCmd.Flags().StringVar(&doctorServer, "server", clientCfg.ServerAddr, "Control plane server address (host:port)")
	doctorCmd.Flags().StringVar(&doctorTarget, "target", clientCfg.LocalTarget, "Local upstream target to probe (host:port)")
	doctorCmd.Flags().StringVar(&doctorInspector, "inspector-addr", clientCfg.InspectorAddr, "Address for local request inspector UI")
	doctorCmd.Flags().BoolVar(&doctorInsecure, "insecure", clientCfg.InsecureSkipVerify, "Allow untrusted or self-signed TLS certificates")
	doctorCmd.Flags().BoolVar(&doctorJSON, "json", false, "Output diagnostic report as JSON")
	doctorCmd.Flags().DurationVar(&doctorTimeout, "timeout", 5*time.Second, "Timeout per diagnostic network probe")

	rootCmd.AddCommand(httpCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(completionCmd)
	rootCmd.AddCommand(doctorCmd)
	statusCmd.Flags().StringVar(&statusAddr, "addr", telemetry.DefaultStatusAddr, "Local agent status address")
	statusCmd.Flags().StringVar(&statusToken, "token", "", "Bearer token for the local status endpoint")
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "Print status as JSON")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
