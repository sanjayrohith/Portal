package inspector

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestReplayEndpoint(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("upstream method = %s, want POST", r.Method)
		}
		if r.URL.String() != "/callback?source=test" {
			t.Errorf("upstream URL = %s, want /callback?source=test", r.URL.String())
		}
		if got := r.Header.Get("X-Replay-Test"); got != "yes" {
			t.Errorf("upstream header = %q, want yes", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read upstream body: %v", err)
		}
		if string(body) != `{"event":"created"}` {
			t.Errorf("upstream body = %q, want captured body", body)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("accepted"))
	}))
	defer upstream.Close()

	buffer := NewRingBuffer(2)
	buffer.Add(&CapturedTransaction{
		ID: "replay-1",
		Request: CapturedRequest{
			Method:  http.MethodPost,
			URL:     "/callback?source=test",
			Path:    "/callback?source=test",
			Headers: HeaderValues{"X-Replay-Test": {"yes"}},
			Body:    []byte(`{"event":"created"}`),
		},
	})
	server, err := NewServerWithReplayTarget(buffer, upstream.URL)
	if err != nil {
		t.Fatal(err)
	}

	response := serveRequest(server.Handler(), http.MethodPost, "/api/requests/replay-1/replay")
	if response.Code != http.StatusOK {
		t.Fatalf("replay status = %d, body = %s", response.Code, response.Body.String())
	}
	var result ReplayResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode replay response: %v", err)
	}
	if result.RequestID != "replay-1" || result.StatusCode != http.StatusAccepted || string(result.Body) != "accepted" {
		t.Fatalf("unexpected replay result: %#v", result)
	}
}

func TestRequestReplayErrors(t *testing.T) {
	buffer := NewRingBuffer(1)
	buffer.Add(&CapturedTransaction{ID: "request-1", Request: CapturedRequest{Method: http.MethodGet, Path: "/"}})
	handler := NewServer(buffer).Handler()

	response := serveRequest(handler, http.MethodPost, "/api/requests/missing/replay")
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing replay status = %d, want 404", response.Code)
	}

	response = serveRequest(handler, http.MethodPost, "/api/requests/request-1/replay")
	if response.Code != http.StatusBadGateway {
		t.Fatalf("unconfigured replay status = %d, want 502", response.Code)
	}
}
