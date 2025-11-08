package resilience

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sanjayrohith/portal/pkg/auth"
	"github.com/sanjayrohith/portal/pkg/control"
	"github.com/sanjayrohith/portal/pkg/mux"
	"github.com/sanjayrohith/portal/pkg/proxy"
	"github.com/sanjayrohith/portal/pkg/registry"
)

func TestEndToEndReconnectRecoversHTTPStreams(t *testing.T) {
	var requests atomic.Int32
	firstRequestReceived := make(chan struct{})
	allowFirstResponse := make(chan struct{})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			close(firstRequestReceived)
			<-allowFirstResponse
		}
		w.Header().Set("X-Portal-Recovered", "true")
		_, _ = io.WriteString(w, "recovered")
	}))
	defer upstream.Close()

	store := auth.NewMemoryTokenStore()
	const rawToken = "e2e-reconnect-token"
	if _, err := store.RegisterRawToken(rawToken, "e2e-owner", 5, 100); err != nil {
		t.Fatalf("register token: %v", err)
	}
	registry := registry.NewSubdomainRegistry()
	authorizer := control.NewPersistentReclamationAuthorizer(store, registry, "tunnel.portal.dev", nil)
	bridge := proxy.NewStreamBridge(
		proxy.NewLocalDialer(upstream.Listener.Addr().String(), 2*time.Second),
		nil,
		2*time.Second,
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type tunnel struct {
		transport net.Conn
		edge      *mux.Session
		client    *mux.Session
	}
	connect := func(clientID string) tunnel {
		serverConn, clientConn := net.Pipe()
		serverHandshakeErr := make(chan error, 1)
		go func() {
			_, _, err := control.ServerHandshake(serverConn, authorizer, time.Second)
			serverHandshakeErr <- err
		}()

		hello := control.ClientHello{
			ClientID:  clientID,
			AuthToken: rawToken,
			Subdomain: "recovery-app",
		}
		serverHello, err := control.ClientHandshake(clientConn, hello, time.Second)
		if err != nil {
			t.Fatalf("client handshake: %v", err)
		}
		if err := <-serverHandshakeErr; err != nil {
			t.Fatalf("server handshake: %v", err)
		}
		if serverHello.AssignedSubdomain != "recovery-app" {
			t.Fatalf("expected recovery-app, got %q", serverHello.AssignedSubdomain)
		}

		serverSession := mux.NewSession(serverConn, true)
		if _, err := registry.Allocate(
			serverHello.AssignedSubdomain,
			auth.HashToken(rawToken),
			"e2e-owner",
			serverSession,
		); err != nil {
			t.Fatalf("bind server session: %v", err)
		}
		clientSession := mux.NewSession(clientConn, false)
		go func() {
			for {
				stream, err := clientSession.AcceptStream()
				if err != nil {
					return
				}
				go func(stream net.Conn) {
					_ = bridge.BridgeStream(ctx, stream)
				}(stream)
			}
		}()
		return tunnel{transport: clientConn, edge: serverSession, client: clientSession}
	}

	initial := connect("e2e-client-1")
	defer initial.edge.Close()
	defer initial.client.Close()

	stalled, err := initial.edge.OpenStream()
	if err != nil {
		t.Fatalf("open initial stream: %v", err)
	}
	request, err := http.NewRequest(http.MethodGet, "http://recovery-app.tunnel.portal.dev/in-flight", nil)
	if err != nil {
		t.Fatalf("create initial request: %v", err)
	}
	if err := proxy.WriteHTTPRequest(stalled, request); err != nil {
		t.Fatalf("write initial request: %v", err)
	}
	select {
	case <-firstRequestReceived:
	case <-time.After(time.Second):
		t.Fatal("upstream did not receive the initial request")
	}

	// A transport loss terminates the old session and its in-flight stream.
	_ = initial.transport.Close()
	close(allowFirstResponse)
	deadline := time.Now().Add(time.Second)
	for !initial.client.IsClosed() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !initial.client.IsClosed() {
		t.Fatal("client session did not detect the dropped transport")
	}

	reconnected := connect("e2e-client-2")
	defer reconnected.edge.Close()
	defer reconnected.client.Close()
	active, ok := registry.GetSession("recovery-app")
	if !ok || active != reconnected.edge || active.IsClosed() {
		t.Fatal("reconnect did not restore the active stable-subdomain session")
	}

	recovered, err := reconnected.edge.OpenStream()
	if err != nil {
		t.Fatalf("open recovered stream: %v", err)
	}
	request, err = http.NewRequest(http.MethodGet, "http://recovery-app.tunnel.portal.dev/recovered", nil)
	if err != nil {
		t.Fatalf("create recovered request: %v", err)
	}
	if err := proxy.WriteHTTPRequest(recovered, request); err != nil {
		t.Fatalf("write recovered request: %v", err)
	}
	response, err := proxy.ReadHTTPResponse(recovered, request)
	if err != nil {
		t.Fatalf("read recovered response: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read recovered body: %v", err)
	}
	if response.StatusCode != http.StatusOK || string(body) != "recovered" {
		t.Fatalf("unexpected recovered response: status=%d body=%q", response.StatusCode, body)
	}
	if requests.Load() != 2 {
		t.Fatalf("expected initial and recovered requests, got %d", requests.Load())
	}
}
