package control

import (
	"crypto/tls"
	"errors"
	"net"
	"testing"
	"time"

	portalErr "github.com/sanjayrohith/portal/pkg/errors"
	"github.com/sanjayrohith/portal/pkg/transport"
)

func TestHandshakeSuccessOverTLS(t *testing.T) {
	cert, err := transport.GenerateSelfSignedCert([]string{"localhost", "127.0.0.1"})
	if err != nil {
		t.Fatalf("failed to generate cert: %v", err)
	}

	listener, err := tls.Listen("tcp", "127.0.0.1:0", transport.ServerTLSConfig(cert))
	if err != nil {
		t.Fatalf("tls.Listen failed: %v", err)
	}
	defer listener.Close()

	serverAddr := listener.Addr().String()

	serverErrCh := make(chan error, 1)
	clientErrCh := make(chan error, 1)

	// Server goroutine
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverErrCh <- err
			return
		}
		defer conn.Close()

		cHello, sHello, err := ServerHandshake(conn, DefaultServerAuthorizer("tunnel.dev"), 2*time.Second)
		if err != nil {
			serverErrCh <- err
			return
		}

		if cHello.Subdomain != "mywebhook" {
			t.Errorf("server: expected subdomain 'mywebhook', got %q", cHello.Subdomain)
		}
		if sHello.AssignedSubdomain != "mywebhook" {
			t.Errorf("server: expected assigned subdomain 'mywebhook', got %q", sHello.AssignedSubdomain)
		}

		tlsConn := conn.(*tls.Conn)
		if tlsConn.ConnectionState().NegotiatedProtocol != transport.ALPNProtocol {
			t.Errorf("server ALPN mismatch: %q", tlsConn.ConnectionState().NegotiatedProtocol)
		}

		serverErrCh <- nil
	}()

	// Client goroutine
	go func() {
		clientCfg := transport.ClientTLSConfig("localhost", true, nil)
		conn, err := tls.Dial("tcp", serverAddr, clientCfg)
		if err != nil {
			clientErrCh <- err
			return
		}
		defer conn.Close()

		hello := ClientHello{
			Version:      CurrentProtocolVersion,
			ClientID:     "client-uuid-1234",
			Subdomain:    "mywebhook",
			Capabilities: []string{"mux-v1", "flow-control"},
		}

		sHello, err := ClientHandshake(conn, hello, 2*time.Second)
		if err != nil {
			clientErrCh <- err
			return
		}

		if sHello.StatusCode != portalErr.StatusSuccess {
			t.Errorf("client: expected StatusSuccess, got %v", sHello.StatusCode)
		}
		if sHello.AssignedSubdomain != "mywebhook" {
			t.Errorf("client: expected assigned subdomain 'mywebhook', got %q", sHello.AssignedSubdomain)
		}
		if sHello.PublicURL != "https://mywebhook.tunnel.dev" {
			t.Errorf("client: expected public URL 'https://mywebhook.tunnel.dev', got %q", sHello.PublicURL)
		}

		clientErrCh <- nil
	}()

	if err := <-clientErrCh; err != nil {
		t.Fatalf("client handshake failed: %v", err)
	}
	if err := <-serverErrCh; err != nil {
		t.Fatalf("server handshake failed: %v", err)
	}
}

func TestHandshakeProtocolMismatchRejection(t *testing.T) {
	cert, err := transport.GenerateSelfSignedCert([]string{"localhost", "127.0.0.1"})
	if err != nil {
		t.Fatalf("cert gen failed: %v", err)
	}

	listener, err := tls.Listen("tcp", "127.0.0.1:0", transport.ServerTLSConfig(cert))
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer listener.Close()

	serverAddr := listener.Addr().String()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		_, _, _ = ServerHandshake(conn, nil, 2*time.Second)
	}()

	clientCfg := transport.ClientTLSConfig("localhost", true, nil)
	conn, err := tls.Dial("tcp", serverAddr, clientCfg)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	// Incompatible client version: 2.0.0 vs server 1.0.0
	incompatibleHello := ClientHello{
		Version:   "2.0.0",
		ClientID:  "test-client",
		Subdomain: "myapp",
	}

	_, err = ClientHandshake(conn, incompatibleHello, 2*time.Second)
	if err == nil {
		t.Fatalf("expected protocol mismatch error, got nil")
	}
	if !errors.Is(err, portalErr.ErrProtocolMismatch) && err.Error() == "" {
		t.Fatalf("expected ErrProtocolMismatch, got %v", err)
	}
}

func TestHandshakeTimeout(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Sleep and do not respond
		time.Sleep(200 * time.Millisecond)
	}()

	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	hello := ClientHello{
		Version:   CurrentProtocolVersion,
		ClientID:  "timeout-client",
		Subdomain: "timeout",
	}

	_, err = ClientHandshake(conn, hello, 50*time.Millisecond)
	if err == nil {
		t.Fatalf("expected handshake timeout error, got nil")
	}
}

func TestHandshakeAuthorizerRejection(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	rejectionErr := portalErr.New(portalErr.StatusSubdomainTaken, "subdomain already reserved", nil)

	go func() {
		authorizer := func(cHello *ClientHello) (*ServerHello, error) {
			return &ServerHello{
				Version:      CurrentProtocolVersion,
				StatusCode:   portalErr.StatusSubdomainTaken,
				ErrorMessage: "subdomain already reserved",
			}, rejectionErr
		}
		_, _, _ = ServerHandshake(serverConn, authorizer, 2*time.Second)
	}()

	hello := ClientHello{
		Version:   CurrentProtocolVersion,
		Subdomain: "conflict-subdomain",
	}

	_, err := ClientHandshake(clientConn, hello, 2*time.Second)
	if err == nil {
		t.Fatalf("expected handshake error on authorizer rejection, got nil")
	}

	if !portalErr.IsSubdomainTaken(err) {
		t.Errorf("expected IsSubdomainTaken(err) to be true, got %v", err)
	}
}
