package edge

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRenderNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	RenderNotFound(rec, "myapp", "portal.dev", "no session active")

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("expected text/html content type, got %s", contentType)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Tunnel Not Online") {
		t.Errorf("expected 'Tunnel Not Online' in body")
	}
	if !strings.Contains(body, "myapp.portal.dev") {
		t.Errorf("expected 'myapp.portal.dev' in body")
	}
	if !strings.Contains(body, "portal http 3000 --subdomain myapp") {
		t.Errorf("expected CLI instructions in body")
	}
}
