// Package telemetry contains Portal's Prometheus metrics and local status APIs.
package telemetry

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics is the process-wide telemetry registry for tunnel activity.
type Metrics struct {
	Registry prometheus.Gatherer

	activeTunnels     prometheus.Gauge
	concurrentStreams prometheus.Gauge
	ingressBytes      *prometheus.CounterVec
	egressBytes       *prometheus.CounterVec
	hopLatency        *prometheus.HistogramVec
	reconnections     prometheus.Counter
	disconnects       *prometheus.CounterVec
}

// NewMetrics creates and registers Portal's core tunnel metrics. A nil
// registerer creates an isolated registry suitable for a daemon instance or
// tests.
func NewMetrics(registerer prometheus.Registerer) (*Metrics, error) {
	if registerer == nil {
		registerer = prometheus.NewRegistry()
	}
	metrics := &Metrics{
		activeTunnels: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "portal",
			Name:      "active_tunnels",
			Help:      "Current number of active client tunnels.",
		}),
		concurrentStreams: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "portal",
			Name:      "concurrent_streams",
			Help:      "Current number of concurrent multiplexed streams.",
		}),
		ingressBytes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "portal",
			Name:      "ingress_bytes_total",
			Help:      "Total bytes received from public ingress by tunnel.",
		}, []string{"tunnel"}),
		egressBytes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "portal",
			Name:      "egress_bytes_total",
			Help:      "Total bytes sent to public egress by tunnel.",
		}, []string{"tunnel"}),
		hopLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "portal",
			Name:      "tunnel_hop_latency_seconds",
			Help:      "Time added by tunnel forwarding for a request hop.",
			Buckets:   []float64{.001, .005, .010, .025, .030, .050, .100, .250, .500, 1},
		}, []string{"tunnel"}),
		reconnections: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "portal",
			Name:      "reconnections_total",
			Help:      "Total number of successful tunnel reconnections.",
		}),
		disconnects: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "portal",
			Name:      "disconnects_total",
			Help:      "Total number of tunnel disconnects by reason.",
		}, []string{"reason"}),
	}

	collectors := []prometheus.Collector{metrics.activeTunnels, metrics.concurrentStreams, metrics.ingressBytes, metrics.egressBytes, metrics.hopLatency, metrics.reconnections, metrics.disconnects}
	for _, collector := range collectors {
		if err := registerer.Register(collector); err != nil {
			return nil, err
		}
	}
	if gatherer, ok := registerer.(prometheus.Gatherer); ok {
		metrics.Registry = gatherer
	}
	return metrics, nil
}

// RecordReconnection increments the successful reconnect counter.
func (m *Metrics) RecordReconnection() { m.reconnections.Inc() }

// RecordDisconnect increments the disconnect counter for a normalized reason.
func (m *Metrics) RecordDisconnect(reason string) {
	if reason == "" {
		reason = "unknown"
	}
	m.disconnects.WithLabelValues(reason).Inc()
}

// ObserveHopLatency records the forwarding duration for a tunnel hop.
func (m *Metrics) ObserveHopLatency(tunnel string, duration time.Duration) {
	if duration < 0 {
		duration = 0
	}
	m.hopLatency.WithLabelValues(tunnel).Observe(duration.Seconds())
}

// SetActiveTunnels records the current active tunnel count.
func (m *Metrics) SetActiveTunnels(count int) { m.activeTunnels.Set(float64(count)) }

// SetConcurrentStreams records the current multiplexed stream count.
func (m *Metrics) SetConcurrentStreams(count int) { m.concurrentStreams.Set(float64(count)) }

// AddIngressBytes records bytes received from public ingress for a tunnel.
func (m *Metrics) AddIngressBytes(tunnel string, count int64) {
	m.ingressBytes.WithLabelValues(tunnel).Add(float64(count))
}

// AddEgressBytes records bytes sent to public egress for a tunnel.
func (m *Metrics) AddEgressBytes(tunnel string, count int64) {
	m.egressBytes.WithLabelValues(tunnel).Add(float64(count))
}
