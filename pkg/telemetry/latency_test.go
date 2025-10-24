package telemetry

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestMetricsObserveHopLatency(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := NewMetrics(registry)
	if err != nil {
		t.Fatal(err)
	}
	metrics.ObserveHopLatency("alpha", 12*time.Millisecond)
	duration := metrics.TrackHop("alpha", time.Now().Add(-2*time.Millisecond))
	if duration < 2*time.Millisecond {
		t.Fatalf("TrackHop() duration = %v, want at least 2ms", duration)
	}

	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() == "portal_tunnel_hop_latency_seconds" {
			if len(family.Metric) != 1 || family.Metric[0].GetHistogram().GetSampleCount() != 2 {
				t.Fatalf("latency histogram = %#v, want two observations", family)
			}
			return
		}
	}
	t.Fatal("latency histogram was not gathered")
}
