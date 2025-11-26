package doctor

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/sanjayrohith/portal/pkg/transport"
)

// CheckStatus defines the outcome of an individual diagnostic check.
type CheckStatus string

const (
	StatusPass CheckStatus = "PASS"
	StatusWarn CheckStatus = "WARN"
	StatusFail CheckStatus = "FAIL"
)

// CheckItem represents the telemetry and outcome of a single diagnostic probe.
type CheckItem struct {
	Category string      `json:"category"`
	Name     string      `json:"name"`
	Status   CheckStatus `json:"status"`
	Duration int64       `json:"duration_ms"`
	Message  string      `json:"message"`
	Remedy   string      `json:"remedy,omitempty"`
}

// Report aggregates all diagnostic probe results into a comprehensive summary.
type Report struct {
	Timestamp time.Time   `json:"timestamp"`
	Target    string      `json:"target"`
	Server    string      `json:"server"`
	AllPassed bool        `json:"all_passed"`
	Checks    []CheckItem `json:"checks"`
}

// Options configures the probe parameters for RunDiagnostics.
type Options struct {
	ServerAddr         string
	LocalTarget        string
	InspectorAddr      string
	InsecureSkipVerify bool
	Timeout            time.Duration
	RootCAs            *x509.CertPool
}

// DefaultOptions returns default diagnostic probe parameters.
func DefaultOptions() Options {
	return Options{
		ServerAddr:         "127.0.0.1:8443",
		LocalTarget:        "127.0.0.1:3000",
		InspectorAddr:      "127.0.0.1:4040",
		InsecureSkipVerify: false,
		Timeout:            5 * time.Second,
	}
}

// RunDiagnostics executes a full battery of environment, DNS, TCP, and TLS checks.
func RunDiagnostics(opts Options) Report {
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Second
	}
	if opts.InspectorAddr == "" {
		opts.InspectorAddr = "127.0.0.1:4040"
	}

	report := Report{
		Timestamp: time.Now().UTC(),
		Target:    opts.LocalTarget,
		Server:    opts.ServerAddr,
		AllPassed: true,
		Checks:    make([]CheckItem, 0, 6),
	}

	// 1. Loopback networking probe
	report.addCheck(CheckLoopback(opts.Timeout))

	// 2. Inspector port binding availability probe
	report.addCheck(CheckInspectorPort(opts.InspectorAddr))

	// 3. Local upstream service connectivity probe
	if opts.LocalTarget != "" {
		report.addCheck(CheckLocalTarget(opts.LocalTarget, opts.Timeout))
	}

	// Parse host & port for remote checks
	host, port, err := net.SplitHostPort(opts.ServerAddr)
	if err != nil {
		host = opts.ServerAddr
		port = "8443"
	}

	// 4. DNS resolution probe
	dnsCheck, ips := CheckDNS(host, opts.Timeout)
	report.addCheck(dnsCheck)

	// Only attempt TCP and TLS if DNS resolution succeeded or IP was provided
	if dnsCheck.Status != StatusFail {
		targetAddr := net.JoinHostPort(host, port)
		if len(ips) > 0 {
			targetAddr = net.JoinHostPort(ips[0].String(), port)
		}

		// 5. TCP transport connectivity probe
		tcpCheck := CheckTCP(targetAddr, opts.Timeout)
		report.addCheck(tcpCheck)

		// 6. TLS handshake probe
		if tcpCheck.Status == StatusPass {
			tlsCheck := CheckTLS(targetAddr, host, opts.InsecureSkipVerify, opts.RootCAs, opts.Timeout)
			report.addCheck(tlsCheck)
		}
	}

	for _, c := range report.Checks {
		if c.Status == StatusFail {
			report.AllPassed = false
			break
		}
	}

	return report
}

func (r *Report) addCheck(c CheckItem) {
	r.Checks = append(r.Checks, c)
}

// CheckLoopback verifies that the local loopback interface is active and bindable.
func CheckLoopback(timeout time.Duration) CheckItem {
	start := time.Now()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	dur := time.Since(start).Milliseconds()
	if err != nil {
		return CheckItem{
			Category: "Environment",
			Name:     "Loopback Interface",
			Status:   StatusFail,
			Duration: dur,
			Message:  fmt.Sprintf("Failed to bind to 127.0.0.1: %v", err),
			Remedy:   "Ensure 127.0.0.1 loopback interface (lo) is up: ip link set lo up",
		}
	}
	_ = ln.Close()
	return CheckItem{
		Category: "Environment",
		Name:     "Loopback Interface",
		Status:   StatusPass,
		Duration: dur,
		Message:  "127.0.0.1 loopback networking is functional",
	}
}

// CheckInspectorPort verifies that the local web inspector port is free to bind.
func CheckInspectorPort(addr string) CheckItem {
	start := time.Now()
	ln, err := net.Listen("tcp", addr)
	dur := time.Since(start).Milliseconds()
	if err != nil {
		return CheckItem{
			Category: "Environment",
			Name:     "Inspector Port Availability",
			Status:   StatusWarn,
			Duration: dur,
			Message:  fmt.Sprintf("Inspector address %s is currently in use: %v", addr, err),
			Remedy:   "Another process or existing portal instance is running on this port; pass --inspector-addr with a free port",
		}
	}
	_ = ln.Close()
	return CheckItem{
		Category: "Environment",
		Name:     "Inspector Port Availability",
		Status:   StatusPass,
		Duration: dur,
		Message:  fmt.Sprintf("Port %s is available for web inspector UI", addr),
	}
}

// CheckLocalTarget checks whether the developer's target HTTP application is listening.
func CheckLocalTarget(target string, timeout time.Duration) CheckItem {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", target, timeout)
	dur := time.Since(start).Milliseconds()
	if err != nil {
		return CheckItem{
			Category: "Environment",
			Name:     "Local Upstream Target",
			Status:   StatusWarn,
			Duration: dur,
			Message:  fmt.Sprintf("No local service listening on %s (%v)", target, err),
			Remedy:   "Start your local development server (e.g. npm run dev, go run main.go) on this port",
		}
	}
	_ = conn.Close()
	return CheckItem{
		Category: "Environment",
		Name:     "Local Upstream Target",
		Status:   StatusPass,
		Duration: dur,
		Message:  fmt.Sprintf("Local service listening on %s", target),
	}
}

// CheckDNS probes DNS resolution for the specified control plane hostname.
func CheckDNS(host string, timeout time.Duration) (CheckItem, []net.IP) {
	start := time.Now()
	// If it's already an IP, pass immediately
	if ip := net.ParseIP(host); ip != nil {
		dur := time.Since(start).Milliseconds()
		return CheckItem{
			Category: "DNS",
			Name:     "Host Resolution",
			Status:   StatusPass,
			Duration: dur,
			Message:  fmt.Sprintf("Direct IP address supplied: %s", host),
		}, []net.IP{ip}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resolver := net.DefaultResolver
	addrs, err := resolver.LookupIPAddr(ctx, host)
	dur := time.Since(start).Milliseconds()
	if err != nil {
		return CheckItem{
			Category: "DNS",
			Name:     "Host Resolution",
			Status:   StatusFail,
			Duration: dur,
			Message:  fmt.Sprintf("Failed to resolve hostname %s: %v", host, err),
			Remedy:   fmt.Sprintf("Verify that %s has valid DNS A/AAAA records configured in your DNS provider", host),
		}, nil
	}

	ips := make([]net.IP, 0, len(addrs))
	ipStrs := make([]string, 0, len(addrs))
	for _, a := range addrs {
		ips = append(ips, a.IP)
		ipStrs = append(ipStrs, a.IP.String())
	}

	return CheckItem{
		Category: "DNS",
		Name:     "Host Resolution",
		Status:   StatusPass,
		Duration: dur,
		Message:  fmt.Sprintf("Resolved %s -> [%s]", host, strings.Join(ipStrs, ", ")),
	}, ips
}

// CheckTCP probes raw TCP connectivity to the control plane port.
func CheckTCP(addr string, timeout time.Duration) CheckItem {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, timeout)
	dur := time.Since(start).Milliseconds()
	if err != nil {
		return CheckItem{
			Category: "Network",
			Name:     "TCP Connection",
			Status:   StatusFail,
			Duration: dur,
			Message:  fmt.Sprintf("Failed to connect to %s over TCP: %v", addr, err),
			Remedy:   fmt.Sprintf("Check that portald is running and firewall allows inbound TCP on %s", addr),
		}
	}
	_ = conn.Close()
	return CheckItem{
		Category: "Network",
		Name:     "TCP Connection",
		Status:   StatusPass,
		Duration: dur,
		Message:  fmt.Sprintf("TCP connection established to %s", addr),
	}
}

// CheckTLS executes a mutual TLS 1.3 handshake and validates certificate health.
func CheckTLS(addr, serverName string, insecure bool, rootCAs *x509.CertPool, timeout time.Duration) CheckItem {
	start := time.Now()
	tlsConfig := transport.ClientTLSConfig(serverName, insecure, rootCAs)

	dialer := &net.Dialer{Timeout: timeout}
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
	dur := time.Since(start).Milliseconds()
	if err != nil {
		return CheckItem{
			Category: "Security",
			Name:     "TLS Handshake",
			Status:   StatusFail,
			Duration: dur,
			Message:  fmt.Sprintf("TLS handshake failed with %s: %v", addr, err),
			Remedy:   "Ensure server certificate matches hostname and is signed by a trusted CA, or pass --insecure for self-signed certificates",
		}
	}
	defer conn.Close()

	state := conn.ConnectionState()
	alpn := state.NegotiatedProtocol
	if alpn != transport.ALPNProtocol {
		return CheckItem{
			Category: "Security",
			Name:     "TLS Handshake",
			Status:   StatusWarn,
			Duration: dur,
			Message:  fmt.Sprintf("Negotiated unexpected ALPN protocol %q (expected %q)", alpn, transport.ALPNProtocol),
			Remedy:   "Ensure control plane portald is running with current protocol version",
		}
	}

	var certDesc string
	if len(state.PeerCertificates) > 0 {
		cert := state.PeerCertificates[0]
		remaining := time.Until(cert.NotAfter)
		days := int(remaining.Hours() / 24)
		certDesc = fmt.Sprintf(" (Cert valid for %d days, CN=%s)", days, cert.Subject.CommonName)
	}

	return CheckItem{
		Category: "Security",
		Name:     "TLS Handshake",
		Status:   StatusPass,
		Duration: dur,
		Message:  fmt.Sprintf("TLS 1.3 negotiated with %s [ALPN=%s]%s", serverName, alpn, certDesc),
	}
}

// FormatText renders the diagnostic report as human-readable CLI text.
func FormatText(report Report) string {
	var sb strings.Builder
	sb.WriteString("=================================================================\n")
	sb.WriteString("                     PORTAL DOCTOR REPORT                        \n")
	sb.WriteString("=================================================================\n")
	sb.WriteString(fmt.Sprintf("Timestamp: %s\n", report.Timestamp.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("Server:    %s\n", report.Server))
	if report.Target != "" {
		sb.WriteString(fmt.Sprintf("Target:    %s\n", report.Target))
	}
	sb.WriteString("-----------------------------------------------------------------\n\n")

	for _, check := range report.Checks {
		var icon string
		switch check.Status {
		case StatusPass:
			icon = "[PASS]"
		case StatusWarn:
			icon = "[WARN]"
		case StatusFail:
			icon = "[FAIL]"
		}

		sb.WriteString(fmt.Sprintf("%-6s %-12s %-28s (%dms)\n", icon, check.Category, check.Name, check.Duration))
		sb.WriteString(fmt.Sprintf("       %s\n", check.Message))
		if check.Remedy != "" {
			sb.WriteString(fmt.Sprintf("       Remedy: %s\n", check.Remedy))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("-----------------------------------------------------------------\n")
	if report.AllPassed {
		sb.WriteString("Overall Status: HEALTHY — environment is fully ready for tunneling!\n")
	} else {
		sb.WriteString("Overall Status: ATTENTION NEEDED — resolve failed checks above.\n")
	}
	sb.WriteString("=================================================================\n")

	return sb.String()
}

// FormatJSON renders the diagnostic report as structured JSON.
func FormatJSON(report Report) (string, error) {
	bytes, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
