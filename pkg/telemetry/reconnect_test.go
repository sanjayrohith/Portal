package telemetry

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestMetricsReconnectAndDisconnectCounters(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := NewMetrics(registry)
	if err != nil {
		t.Fatal(err)
	}
	metrics.RecordReconnection()
	metrics.RecordReconnection()
	metrics.RecordDisconnect("network_drop")
	metrics.RecordDisconnect("")

	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	seenReconnect, seenNetwork, seenUnknown := false, false, false
	for _, family := range families {
		switch family.GetName() {
		case "portal_reconnections_total":
			seenReconnect = family.Metric[0].GetCounter().GetValue() == 2
		case "portal_disconnects_total":
			for _, metric := range family.Metric {
				for _, label := range metric.Label {
					if label.GetName() == "reason" && label.GetValue() == "network_drop" {
						seenNetwork = true
					}
					if label.GetName() == "reason" && label.GetValue() == "unknown" {
						seenUnknown = true
					}
				}
			}
		}
	}
	if !seenReconnect || !seenNetwork || !seenUnknown {
		t.Fatalf("counters missing or incorrect: reconnect=%v network=%v unknown=%v", seenReconnect, seenNetwork, seenUnknown)
	}
}
