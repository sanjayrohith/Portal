package inspector

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRequestListAndDetailEndpoints(t *testing.T) {
	buffer := NewRingBuffer(4)
	started := time.Date(2025, time.October, 17, 12, 0, 0, 0, time.UTC)
	buffer.Add(&CapturedTransaction{
		ID:        "request-1",
		StartedAt: started,
		Duration:  25 * time.Millisecond,
		ClientIP:  "127.0.0.1",
		Request: CapturedRequest{
			Method: "POST",
			Path:   "/hooks",
			Body:   []byte(`{"ok":true}`),
		},
		Response: &CapturedResponse{StatusCode: http.StatusCreated, Body: []byte("created")},
	})
	server := NewServer(buffer)

	listResponse := serveRequest(server.Handler(), http.MethodGet, "/api/requests")
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200", listResponse.Code)
	}
	var summaries []RequestSummary
	if err := json.Unmarshal(listResponse.Body.Bytes(), &summaries); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(summaries) != 1 || summaries[0].ID != "request-1" || summaries[0].StatusCode != http.StatusCreated {
		t.Fatalf("unexpected summaries: %#v", summaries)
	}

	detailResponse := serveRequest(server.Handler(), http.MethodGet, "/api/requests/request-1")
	if detailResponse.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want 200", detailResponse.Code)
	}
	var transaction CapturedTransaction
	if err := json.Unmarshal(detailResponse.Body.Bytes(), &transaction); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if transaction.ID != "request-1" || string(transaction.Request.Body) != `{"ok":true}` {
		t.Fatalf("unexpected transaction: %#v", transaction)
	}
	if got := detailResponse.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("detail content type = %q, want application/json", got)
	}
}

func TestRequestEndpointsErrorsAndMethods(t *testing.T) {
	handler := NewServer(NewRingBuffer(1)).Handler()

	for _, test := range []struct {
		method string
		path   string
		status int
	}{
		{http.MethodGet, "/api/requests/missing", http.StatusNotFound},
		{http.MethodPost, "/api/requests", http.StatusMethodNotAllowed},
		{http.MethodPost, "/api/requests/id", http.StatusMethodNotAllowed},
		{http.MethodGet, "/unknown", http.StatusNotFound},
	} {
		response := serveRequest(handler, test.method, test.path)
		if response.Code != test.status {
			t.Errorf("%s %s status = %d, want %d", test.method, test.path, response.Code, test.status)
		}
	}
}

func serveRequest(handler http.Handler, method, path string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}
