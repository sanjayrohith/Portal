package security

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestSizeCapMiddleware(t *testing.T) {
	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "error reading body", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	mw := RequestSizeCapMiddleware(100, dummyHandler)

	t.Run("Allowed small payload", func(t *testing.T) {
		req, _ := http.NewRequest("POST", "/data", bytes.NewBuffer(make([]byte, 50)))
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("Rejected with upfront Content-Length > limit", func(t *testing.T) {
		req, _ := http.NewRequest("POST", "/data", bytes.NewBuffer(make([]byte, 200)))
		req.ContentLength = 200
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)

		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("expected 413, got %d", rec.Code)
		}
	})

	t.Run("Rejected streaming body > limit", func(t *testing.T) {
		req, _ := http.NewRequest("POST", "/data", bytes.NewBuffer(make([]byte, 200)))
		req.ContentLength = -1 // chunked/unknown
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)

		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("expected 413, got %d", rec.Code)
		}
	})
}
