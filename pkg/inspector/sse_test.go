package inspector

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEventsStreamPublishesCapturedTransaction(t *testing.T) {
	server := NewServer(NewRingBuffer(2))
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, httpServer.URL+"/api/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("events response = %d %q, want 200 text/event-stream", response.StatusCode, response.Header.Get("Content-Type"))
	}

	server.Publish(&CapturedTransaction{ID: "live-1", Request: CapturedRequest{Method: http.MethodGet, Path: "/live"}})
	scanner := bufio.NewScanner(response.Body)
	var eventData string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			eventData = strings.TrimPrefix(line, "data: ")
			break
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(eventData, `"id":"live-1"`) {
		t.Fatalf("event data = %q, want live transaction", eventData)
	}

	cancel()
}
