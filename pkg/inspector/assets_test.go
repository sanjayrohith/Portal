package inspector

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedInspectorAssets(t *testing.T) {
	handler := NewServer(NewRingBuffer(1)).Handler()
	for _, test := range []struct {
		path        string
		contentType string
		contains    string
	}{
		{"/", "text/html", "Portal Inspector"},
		{"/static/style.css", "text/css", "--cyan"},
		{"/static/app.js", "text/javascript", "EventSource"},
	} {
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", test.path, response.Code)
		}
		if !strings.Contains(response.Header().Get("Content-Type"), test.contentType) {
			t.Errorf("GET %s content type = %q, want %q", test.path, response.Header().Get("Content-Type"), test.contentType)
		}
		if !strings.Contains(response.Body.String(), test.contains) {
			t.Errorf("GET %s body does not contain %q", test.path, test.contains)
		}
	}
}
