package telemetry

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestMetricsGatherTunnelActivity(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := NewMetrics(registry)
	if err != nil {
		t.Fatal(err)
	}
	metrics.SetActiveTunnels(3)
	metrics.SetConcurrentStreams(7)
	metrics.AddIngressBytes("alpha", 128)
	metrics.AddEgressBytes("alpha", 64)

	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	var exposition strings.Builder
	for _, family := range families {
		exposition.WriteString(family.GetName())
		for _, metric := range family.Metric {
			exposition.WriteString(metric.String())
		}
	}
	for _, name := range []string{"portal_active_tunnels", "portal_concurrent_streams", "portal_ingress_bytes_total", "portal_egress_bytes_total"} {
		if !strings.Contains(exposition.String(), name) {
			t.Errorf("gathered metrics do not contain %s", name)
		}
	}
}
