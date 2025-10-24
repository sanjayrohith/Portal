package telemetry

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// AuthenticatedMetricsHandler exposes Prometheus text metrics only when the
// request carries Authorization: Bearer <token>.
func AuthenticatedMetricsHandler(metrics *Metrics, token string) http.Handler {
	if metrics == nil || metrics.Registry == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "metrics unavailable", http.StatusInternalServerError)
		})
	}
	gatherer, ok := metrics.Registry.(prometheus.Gatherer)
	if !ok {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "metrics unavailable", http.StatusInternalServerError)
		})
	}
	metricsHandler := promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		provided := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if token == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		metricsHandler.ServeHTTP(w, r)
	})
}
