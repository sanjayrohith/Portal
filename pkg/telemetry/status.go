package telemetry

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultStatusAddr is the loopback address used by the local client agent.
	DefaultStatusAddr = "127.0.0.1:4041"
	statusPath        = "/status"
)

// StatusSnapshot contains live client tunnel statistics.
type StatusSnapshot struct {
	ActiveStreams int64         `json:"active_streams"`
	Uptime        time.Duration `json:"uptime"`
	IngressBytes  int64         `json:"ingress_bytes"`
	EgressBytes   int64         `json:"egress_bytes"`
	StartedAt     time.Time     `json:"started_at"`
}

// QueryLocalStatus queries the local client agent's status endpoint.
func QueryLocalStatus(addr, token string) (StatusSnapshot, error) {
	if addr == "" {
		addr = DefaultStatusAddr
	}
	endpoint := addr
	if !strings.Contains(endpoint, "://") {
		endpoint = "http://" + endpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return StatusSnapshot{}, fmt.Errorf("invalid local status address %q", addr)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + statusPath
	request, err := http.NewRequest(http.MethodGet, parsed.String(), nil)
	if err != nil {
		return StatusSnapshot{}, fmt.Errorf("create status request: %w", err)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := (&http.Client{Timeout: 2 * time.Second}).Do(request)
	if err != nil {
		return StatusSnapshot{}, fmt.Errorf("query local status: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return StatusSnapshot{}, fmt.Errorf("local status returned HTTP %d", response.StatusCode)
	}
	var snapshot StatusSnapshot
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&snapshot); err != nil {
		return StatusSnapshot{}, fmt.Errorf("decode local status: %w", err)
	}
	return snapshot, nil
}

// FormatStatus formats a status snapshot for terminal or JSON output.
func FormatStatus(snapshot StatusSnapshot, asJSON bool) (string, error) {
	if asJSON {
		encoded, err := json.MarshalIndent(snapshot, "", "  ")
		if err != nil {
			return "", err
		}
		return string(encoded), nil
	}
	return fmt.Sprintf("Active streams: %d\nUptime: %s\nIngress: %s\nEgress: %s\n",
		snapshot.ActiveStreams,
		snapshot.Uptime.Round(time.Second),
		formatBytes(snapshot.IngressBytes),
		formatBytes(snapshot.EgressBytes)), nil
}

func formatBytes(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KiB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%.1f MiB", float64(bytes)/(1024*1024))
}
