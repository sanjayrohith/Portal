package proxy

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sanjayrohith/portal/pkg/mux"
)

func TestEndToEndHTTPForwarding(t *testing.T) {
	// 1. Start mock local upstream HTTP server
	var receivedHost string
	var receivedBody string

	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHost = r.Host
		bodyBytes, _ := io.ReadAll(r.Body)
		receivedBody = string(bodyBytes)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","received":true}`))
	}))
	defer upstreamServer.Close()

	// 2. Set up local dialer and rewriter
	dialer := NewLocalDialer(upstreamServer.Listener.Addr().String(), 2*time.Second)
	rewriter := NewHostRewriter(RewriteModeLocalhost, "", upstreamServer.Listener.Addr().String())
	bridge := NewStreamBridge(dialer, rewriter, 5*time.Second)

	// 3. Set up in-memory multiplexer session pair between edge and client
	edgeConn, clientConn := net.Pipe()
	defer edgeConn.Close()
	defer clientConn.Close()

	edgeSession := mux.NewSession(edgeConn, true)
	clientSession := mux.NewSession(clientConn, false)
	defer edgeSession.Close()
	defer clientSession.Close()

	// Client loop accepting streams from tunnel and bridging to local upstream
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		for {
			stream, err := clientSession.AcceptStream()
			if err != nil {
				return
			}
			go func(s net.Conn) {
				_ = bridge.BridgeStream(ctx, s)
			}(stream)
		}
	}()

	// 4. Edge opens a stream to forward a POST request
	t.Run("POST request forwarding", func(t *testing.T) {
		stream, err := edgeSession.OpenStream()
		if err != nil {
			t.Fatalf("OpenStream failed: %v", err)
		}
		defer stream.Close()

		postData := `{"msg":"hello portal"}`
		req, err := http.NewRequest("POST", "/api/data", bytes.NewBufferString(postData))
		if err != nil {
			t.Fatalf("NewRequest failed: %v", err)
		}
		req.Host = "custom.portal.dev"
		req.Header.Set("Content-Type", "application/json")
		req.ContentLength = int64(len(postData))

		// Write request through multiplexed stream
		if err := WriteHTTPRequest(stream, req); err != nil {
			t.Fatalf("WriteHTTPRequest failed: %v", err)
		}

		// Read response through multiplexed stream
		resp, err := ReadHTTPResponse(stream, req)
		if err != nil {
			t.Fatalf("ReadHTTPResponse failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", resp.StatusCode)
		}

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("reading resp body failed: %v", err)
		}
		if !bytes.Contains(respBody, []byte(`"status":"ok"`)) {
			t.Errorf("unexpected response body: %s", string(respBody))
		}

		if receivedBody != postData {
			t.Errorf("expected upstream to receive %s, got %s", postData, receivedBody)
		}

		if receivedHost != upstreamServer.Listener.Addr().String() {
			t.Errorf("expected upstream host %s, got %s", upstreamServer.Listener.Addr().String(), receivedHost)
		}
	})

	// 5. Edge opens a stream to forward a GET request
	t.Run("GET request forwarding", func(t *testing.T) {
		stream, err := edgeSession.OpenStream()
		if err != nil {
			t.Fatalf("OpenStream failed: %v", err)
		}
		defer stream.Close()

		req, err := http.NewRequest("GET", "/healthz", nil)
		if err != nil {
			t.Fatalf("NewRequest failed: %v", err)
		}
		req.Host = "custom.portal.dev"

		if err := WriteHTTPRequest(stream, req); err != nil {
			t.Fatalf("WriteHTTPRequest failed: %v", err)
		}

		resp, err := ReadHTTPResponse(stream, req)
		if err != nil {
			t.Fatalf("ReadHTTPResponse failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", resp.StatusCode)
		}
	})
}
