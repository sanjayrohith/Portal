package telemetry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestQueryLocalStatus(t *testing.T) {
	started := time.Now().Add(-5 * time.Minute).UTC().Truncate(time.Second)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" || r.Header.Get("Authorization") != "Bearer local-secret" {
			http.Error(w, "bad request", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(StatusSnapshot{ActiveStreams: 3, Uptime: 5 * time.Minute, IngressBytes: 2048, EgressBytes: 1024, StartedAt: started})
	}))
	defer server.Close()

	snapshot, err := QueryLocalStatus(server.URL, "local-secret")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ActiveStreams != 3 || snapshot.IngressBytes != 2048 || !snapshot.StartedAt.Equal(started) {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	formatted, err := FormatStatus(snapshot, false)
	if err != nil || !strings.Contains(formatted, "Active streams: 3") || !strings.Contains(formatted, "2.0 KiB") {
		t.Fatalf("formatted status = %q, error = %v", formatted, err)
	}
}

func TestQueryLocalStatusRejectsNonOK(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	if _, err := QueryLocalStatus(server.URL, ""); err == nil {
		t.Error("QueryLocalStatus() succeeded for non-OK response")
	}
}
