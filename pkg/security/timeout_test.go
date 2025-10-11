package security

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTimeoutMiddleware(t *testing.T) {
	slowHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(100 * time.Millisecond):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
			return
		}
	})

	mw := TimeoutMiddleware(20*time.Millisecond, slowHandler)

	req, _ := http.NewRequest("GET", "/slow", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusGatewayTimeout {
		t.Errorf("expected 504 Gateway Timeout, got %d", rec.Code)
	}
}

func TestIdleStreamPruner(t *testing.T) {
	pruner := NewIdleStreamPruner(50 * time.Millisecond)

	c1, c2 := net.Pipe()
	defer c2.Close()

	pruner.Register(c1)

	now := time.Now()
	// Immediately: not pruned
	if n := pruner.PruneCloses(now); n != 0 {
		t.Errorf("expected 0 pruned, got %d", n)
	}

	// 100ms later: pruned and closed
	pruned := pruner.PruneCloses(now.Add(100 * time.Millisecond))
	if pruned != 1 {
		t.Errorf("expected 1 connection pruned, got %d", pruned)
	}

	// Connection should be closed
	buf := make([]byte, 1)
	_, err := c1.Read(buf)
	if err == nil {
		t.Errorf("expected read error on pruned connection")
	}
}
