package proxy

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/sanjayrohith/portal/pkg/mux"
)

// oauthUpstream simulates a third-party provider (authorize endpoint) plus the
// developer's local app (callback endpoint) behind one persistent subdomain.
func oauthUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	// Provider: validates the request and redirects back to redirect_uri
	// with an authorization code and the echoed state parameter.
	mux.HandleFunc("/oauth/authorize", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		redirectURI := query.Get("redirect_uri")
		state := query.Get("state")
		if query.Get("client_id") == "" || redirectURI == "" || state == "" {
			http.Error(w, "missing oauth parameters", http.StatusBadRequest)
			return
		}
		if _, err := url.ParseRequestURI(redirectURI); err != nil {
			http.Error(w, "invalid redirect_uri", http.StatusBadRequest)
			return
		}
		target := redirectURI + "?code=authcode_abc123&state=" + url.QueryEscape(state)
		http.Redirect(w, r, target, http.StatusFound)
	})

	// Local app: exchanges the callback query into a session.
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("code") == "" || query.Get("state") == "" {
			http.Error(w, "missing code or state", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"authenticated": true,
			"code":          query.Get("code"),
			"state":         query.Get("state"),
		})
	})

	return httptest.NewServer(mux)
}

// oauthGet issues one raw GET through a fresh tunneled stream and returns the
// response with its fully-read body.
func oauthGet(t *testing.T, edgeSession *mux.Session, host, path string) (*http.Response, []byte) {
	t.Helper()
	stream, err := edgeSession.OpenStream()
	if err != nil {
		t.Fatalf("open oauth stream: %v", err)
	}
	defer stream.Close()

	request, err := http.NewRequest(http.MethodGet, "http://"+host+path, nil)
	if err != nil {
		t.Fatalf("create oauth request: %v", err)
	}
	request.Host = host
	if err := WriteHTTPRequest(stream, request); err != nil {
		t.Fatalf("write oauth request: %v", err)
	}
	response, err := ReadHTTPResponse(stream, request)
	if err != nil {
		t.Fatalf("read oauth response: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read oauth response body: %v", err)
	}
	return response, body
}

func TestEndToEndOAuth2AuthorizationCallback(t *testing.T) {
	upstream := oauthUpstream(t)
	defer upstream.Close()

	edgeConn, clientConn := net.Pipe()
	edgeSession := mux.NewSession(edgeConn, true)
	clientSession := mux.NewSession(clientConn, false)
	defer edgeSession.Close()
	defer clientSession.Close()

	// Persistent subdomain binding: every request in this flow addresses the
	// same stable public host, surviving across distinct tunneled streams.
	const publicHost = "myapp.portal.test"
	bridge := NewStreamBridge(NewLocalDialer(upstream.Listener.Addr().String(), 2*time.Second), nil, 2*time.Second)
	go func() {
		for {
			stream, err := clientSession.AcceptStream()
			if err != nil {
				return
			}
			go func(st *mux.Stream) {
				_ = bridge.BridgeStream(t.Context(), st)
			}(stream)
		}
	}()

	// 1. Start login: hit the provider authorize endpoint through the tunnel.
	state := "csrf_state_xyz"
	authPath := "/oauth/authorize?client_id=portal-dev&redirect_uri=" +
		url.QueryEscape("https://"+publicHost+"/callback") +
		"&response_type=code&state=" + state
	authResp, _ := oauthGet(t, edgeSession, publicHost, authPath)
	if authResp.StatusCode != http.StatusFound {
		t.Fatalf("authorize status = %d, want 302", authResp.StatusCode)
	}
	location := authResp.Header.Get("Location")
	wantLocation := "https://" + publicHost + "/callback?code=authcode_abc123&state=" + state
	if location != wantLocation {
		t.Fatalf("redirect location = %q, want %q", location, wantLocation)
	}

	// 2. Follow the redirect back through the same persistent subdomain.
	callbackURL, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse redirect location: %v", err)
	}
	callbackResp, callbackBody := oauthGet(t, edgeSession, publicHost, callbackURL.RequestURI())
	if callbackResp.StatusCode != http.StatusOK {
		t.Fatalf("callback status = %d, want 200 (body %q)", callbackResp.StatusCode, callbackBody)
	}
	var granted map[string]any
	if err := json.Unmarshal(callbackBody, &granted); err != nil {
		t.Fatalf("decode callback body %q: %v", callbackBody, err)
	}
	if granted["authenticated"] != true || granted["code"] != "authcode_abc123" || granted["state"] != state {
		t.Fatalf("callback grant mismatch: %v", granted)
	}

	// 3. A callback with no code must not authenticate.
	deniedResp, _ := oauthGet(t, edgeSession, publicHost, "/callback?state="+state)
	if deniedResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("codeless callback status = %d, want 400", deniedResp.StatusCode)
	}
}
