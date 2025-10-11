package security

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBasicAuthMiddleware(t *testing.T) {
	val := NewFixedCredentialValidator("admin", "secret123")
	mw := BasicAuthMiddleware(BasicAuthMiddlewareConfig{
		Validator: val,
		Realm:     "Protected-Zone",
	})

	dummyHandler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("access granted"))
	}))

	// Case 1: Missing auth header
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	dummyHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rr.Code)
	}
	if authHeader := rr.Header().Get("WWW-Authenticate"); authHeader != `Basic realm="Protected-Zone"` {
		t.Fatalf("unexpected WWW-Authenticate header: %q", authHeader)
	}

	// Case 2: Wrong scheme
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer somerandomtoken")
	rr = httptest.NewRecorder()
	dummyHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rr.Code)
	}

	// Case 3: Invalid base64
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Basic not!base64!")
	rr = httptest.NewRecorder()
	dummyHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rr.Code)
	}

	// Case 4: Wrong credentials
	badCreds := base64.StdEncoding.EncodeToString([]byte("admin:wrongpass"))
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Basic "+badCreds)
	rr = httptest.NewRecorder()
	dummyHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rr.Code)
	}

	// Case 5: Correct credentials
	goodCreds := base64.StdEncoding.EncodeToString([]byte("admin:secret123"))
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Basic "+goodCreds)
	rr = httptest.NewRecorder()
	dummyHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if rr.Body.String() != "access granted" {
		t.Fatalf("unexpected body: %q", rr.Body.String())
	}
}

func TestMultiCredentialValidator(t *testing.T) {
	val := NewMultiCredentialValidator(map[string]string{
		"alice": "wonderland",
		"bob":   "builder",
	})

	if !val.Validate("alice", "wonderland") {
		t.Errorf("expected alice to validate")
	}
	if !val.Validate("bob", "builder") {
		t.Errorf("expected bob to validate")
	}
	if val.Validate("alice", "wrong") {
		t.Errorf("expected alice with wrong pass to fail")
	}
	if val.Validate("charlie", "password") {
		t.Errorf("expected unknown user to fail")
	}
}
