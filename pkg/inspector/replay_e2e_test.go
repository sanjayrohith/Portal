package inspector

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEndToEndInspectorReplayPreservesRequestFidelity(t *testing.T) {
	const (
		publicHost = "orders.portal.dev"
		requestID  = "replay-fidelity-e2e"
		body       = `{"order_id":"42","action":"ship"}`
	)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("upstream method = %s, want POST", r.Method)
		}
		if r.URL.RequestURI() != "/v1/orders/42?expand=items" {
			t.Errorf("upstream URI = %s, want /v1/orders/42?expand=items", r.URL.RequestURI())
		}
		if r.Host != publicHost {
			t.Errorf("upstream Host = %q, want %q", r.Host, publicHost)
		}
		if got := r.Header.Values("X-Request-Tag"); len(got) != 2 || got[0] != "tenant-a" || got[1] != "audit" {
			t.Errorf("upstream repeated X-Request-Tag = %#v, want tenant-a and audit", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("upstream Content-Type = %q, want application/json", got)
		}
		if got := r.Header.Get("X-Forwarded-Proto"); got != "https" {
			t.Errorf("upstream X-Forwarded-Proto = %q, want https", got)
		}
		gotBody, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read upstream body: %v", err)
		}
		if string(gotBody) != body {
			t.Errorf("upstream body = %q, want %q", gotBody, body)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, "replayed")
	}))
	defer upstream.Close()

	buffer := NewRingBuffer(2)
	buffer.Add(&CapturedTransaction{
		ID: requestID,
		Request: CapturedRequest{
			Method: http.MethodPost,
			URL:    "https://" + publicHost + "/v1/orders/42?expand=items",
			Path:   "/v1/orders/42?expand=items",
			Headers: HeaderValues{
				"Host":              {publicHost},
				"X-Request-Tag":     {"tenant-a", "audit"},
				"Content-Type":      {"application/json"},
				"X-Forwarded-Proto": {"https"},
			},
			Body:      []byte(body),
			Timestamp: time.Now(),
		},
	})

	inspectorServer, err := NewServerWithReplayTarget(buffer, upstream.URL)
	if err != nil {
		t.Fatalf("create inspector server: %v", err)
	}
	api := httptest.NewServer(inspectorServer.Handler())
	defer api.Close()

	replayResponse, err := http.Post(api.URL+"/api/requests/"+requestID+"/replay", "application/json", nil)
	if err != nil {
		t.Fatalf("call replay endpoint: %v", err)
	}
	defer replayResponse.Body.Close()
	if replayResponse.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(replayResponse.Body)
		t.Fatalf("replay status = %d, body = %s", replayResponse.StatusCode, responseBody)
	}

	var result ReplayResult
	if err := json.NewDecoder(replayResponse.Body).Decode(&result); err != nil {
		t.Fatalf("decode replay result: %v", err)
	}
	if result.RequestID != requestID || result.StatusCode != http.StatusAccepted || string(result.Body) != "replayed" {
		t.Fatalf("unexpected replay result: %#v", result)
	}
}
