package telemetry

import "time"

// TrackHop measures and records the elapsed time since started, returning the
// same duration for callers that need to include it in logs or responses.
func (m *Metrics) TrackHop(tunnel string, started time.Time) time.Duration {
	duration := time.Since(started)
	m.ObserveHopLatency(tunnel, duration)
	return duration
}
