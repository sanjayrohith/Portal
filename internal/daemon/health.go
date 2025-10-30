package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/sanjayrohith/portal/pkg/registry"
)

// HealthStatus is the JSON body returned by liveness and readiness probes.
type HealthStatus struct {
	Status   string            `json:"status"`
	Checks   map[string]string `json:"checks,omitempty"`
	Database string            `json:"database,omitempty"`
}

// HealthChecker verifies listener and database health for readiness probes.
// Nil checks are treated as passing so liveness works before backends attach.
type HealthChecker struct {
	ListenerCheck func() error
	StorageCheck  func(ctx context.Context) error
	Timeout       time.Duration
}

func (h *HealthChecker) timeout() time.Duration {
	if h == nil || h.Timeout <= 0 {
		return 2 * time.Second
	}
	return h.Timeout
}

// LivenessHandler always returns 200 while the process is alive. It backs
// the Kubernetes liveness probe (GET /healthz).
func LivenessHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(HealthStatus{Status: "ok"})
	})
}

// ReadinessHandler verifies listeners and storage before reporting ready. It
// backs the Kubernetes/Docker readiness probe (GET /readyz).
func (h *HealthChecker) ReadinessHandler() http.Handler {
	checker := h
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		checks := make(map[string]string)
		ready := true

		if checker == nil || checker.ListenerCheck == nil {
			checks["listeners"] = "ok"
		} else if err := checker.ListenerCheck(); err != nil {
			checks["listeners"] = "fail: " + err.Error()
			ready = false
		} else {
			checks["listeners"] = "ok"
		}

		if checker == nil || checker.StorageCheck == nil {
			checks["database"] = "ok"
		} else {
			ctx, cancel := context.WithTimeout(r.Context(), checker.timeout())
			defer cancel()
			if err := checker.StorageCheck(ctx); err != nil {
				checks["database"] = "fail: " + err.Error()
				ready = false
			} else {
				checks["database"] = "ok"
			}
		}

		status := HealthStatus{Status: "ok", Checks: checks}
		if !ready {
			status.Status = "not-ready"
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	})
}

// NewHealthMux builds a probe-only mux with /healthz and /readyz.
func NewHealthMux(checker *HealthChecker) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/healthz", LivenessHandler())
	var readiness http.Handler = (&HealthChecker{}).ReadinessHandler()
	if checker != nil {
		readiness = checker.ReadinessHandler()
	}
	mux.Handle("/readyz", readiness)
	return mux
}

// AdminMux combines probes with operator APIs served on the admin address.
func AdminMux(reg *registry.SubdomainRegistry, checker *HealthChecker) *http.ServeMux {
	mux := NewHealthMux(checker)
	mux.Handle("/api/tunnels", TunnelsHandler(reg))
	return mux
}
