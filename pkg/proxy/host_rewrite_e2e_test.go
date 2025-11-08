package proxy

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sanjayrohith/portal/pkg/mux"
)

func TestEndToEndHostRewriteForMultiTenantUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body string
		switch r.Host {
		case "tenant-a.local":
			body = "tenant-a"
		case "tenant-b.local":
			body = "tenant-b"
		default:
			http.Error(w, "unknown tenant", http.StatusNotFound)
			return
		}

		w.Header().Set("X-Received-Public-Host", r.Header.Get("X-Forwarded-Host"))
		_, _ = io.WriteString(w, body)
	}))
	defer upstream.Close()

	dialer := NewLocalDialer(upstream.Listener.Addr().String(), 2*time.Second)
	testTenant := func(t *testing.T, publicHost, localHost, expectedBody string) {
		t.Helper()
		edgeConn, clientConn := net.Pipe()
		edgeSession := mux.NewSession(edgeConn, true)
		clientSession := mux.NewSession(clientConn, false)
		defer edgeSession.Close()
		defer clientSession.Close()

		bridge := NewStreamBridge(
			dialer,
			NewHostRewriter(RewriteModeCustom, localHost, ""),
			2*time.Second,
		)
		bridgeDone := make(chan struct{})
		go func() {
			defer close(bridgeDone)
			stream, err := clientSession.AcceptStream()
			if err != nil {
				return
			}
			_ = bridge.BridgeStream(t.Context(), stream)
		}()

		stream, err := edgeSession.OpenStream()
		if err != nil {
			t.Fatalf("open %s stream: %v", publicHost, err)
		}
		request, err := http.NewRequest(http.MethodGet, "http://"+publicHost+"/tenant", nil)
		if err != nil {
			t.Fatalf("create %s request: %v", publicHost, err)
		}
		request.Host = publicHost
		if err := WriteHTTPRequest(stream, request); err != nil {
			t.Fatalf("write %s request: %v", publicHost, err)
		}

		response, err := ReadHTTPResponse(stream, request)
		if err != nil {
			t.Fatalf("read %s response: %v", publicHost, err)
		}
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("read %s response body: %v", publicHost, err)
		}
		if response.StatusCode != http.StatusOK || string(body) != expectedBody {
			t.Fatalf("unexpected %s response: status=%d body=%q", publicHost, response.StatusCode, body)
		}
		if response.Header.Get("X-Received-Public-Host") != publicHost {
			t.Fatalf("upstream received forwarded host %q, want %q", response.Header.Get("X-Received-Public-Host"), publicHost)
		}

		_ = stream.Close()
		select {
		case <-bridgeDone:
		case <-time.After(time.Second):
			t.Fatalf("bridge for %s did not finish", publicHost)
		}
	}

	t.Run("tenant A", func(t *testing.T) {
		testTenant(t, "tenant-a.portal.dev", "tenant-a.local", "tenant-a")
	})
	t.Run("tenant B", func(t *testing.T) {
		testTenant(t, "tenant-b.portal.dev", "tenant-b.local", "tenant-b")
	})
}
