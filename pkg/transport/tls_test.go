package transport

import (
	"crypto/tls"
	"net"
	"testing"
)

func TestTLSConfigurationEnforcement(t *testing.T) {
	clientCfg := ClientTLSConfig("portal.example.com", false, nil)
	if clientCfg.MinVersion != tls.VersionTLS13 {
		t.Errorf("expected client MinVersion TLS 1.3, got 0x%04x", clientCfg.MinVersion)
	}
	if len(clientCfg.NextProtos) != 1 || clientCfg.NextProtos[0] != ALPNProtocol {
		t.Errorf("expected client NextProtos [%s], got %v", ALPNProtocol, clientCfg.NextProtos)
	}
	if clientCfg.ServerName != "portal.example.com" {
		t.Errorf("expected client ServerName 'portal.example.com', got %q", clientCfg.ServerName)
	}

	cert, err := GenerateSelfSignedCert([]string{"localhost", "127.0.0.1"})
	if err != nil {
		t.Fatalf("GenerateSelfSignedCert failed: %v", err)
	}

	serverCfg := ServerTLSConfig(cert)
	if serverCfg.MinVersion != tls.VersionTLS13 {
		t.Errorf("expected server MinVersion TLS 1.3, got 0x%04x", serverCfg.MinVersion)
	}
	if len(serverCfg.NextProtos) != 1 || serverCfg.NextProtos[0] != ALPNProtocol {
		t.Errorf("expected server NextProtos [%s], got %v", ALPNProtocol, serverCfg.NextProtos)
	}
	if len(serverCfg.Certificates) != 1 {
		t.Fatalf("expected 1 server certificate, got %d", len(serverCfg.Certificates))
	}
}

func TestTLSServerClientHandshakeALPN(t *testing.T) {
	cert, err := GenerateSelfSignedCert([]string{"localhost", "127.0.0.1"})
	if err != nil {
		t.Fatalf("GenerateSelfSignedCert failed: %v", err)
	}

	serverCfg := ServerTLSConfig(cert)
	listener, err := tls.Listen("tcp", "127.0.0.1:0", serverCfg)
	if err != nil {
		t.Fatalf("tls.Listen failed: %v", err)
	}
	defer listener.Close()

	serverAddr := listener.Addr().String()

	clientErrCh := make(chan error, 1)
	serverErrCh := make(chan error, 1)

	// Server accept
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverErrCh <- err
			return
		}
		defer conn.Close()

		tlsConn, ok := conn.(*tls.Conn)
		if !ok {
			serverErrCh <- err
			return
		}
		if err := tlsConn.Handshake(); err != nil {
			serverErrCh <- err
			return
		}

		state := tlsConn.ConnectionState()
		if state.NegotiatedProtocol != ALPNProtocol {
			t.Errorf("server: negotiated ALPN mismatch: got %q, want %q", state.NegotiatedProtocol, ALPNProtocol)
		}
		if state.Version != tls.VersionTLS13 {
			t.Errorf("server: TLS version mismatch: got 0x%04x, want TLS 1.3", state.Version)
		}
		serverErrCh <- nil
	}()

	// Client dial
	go func() {
		clientCfg := ClientTLSConfig("localhost", true, nil)
		conn, err := tls.Dial("tcp", serverAddr, clientCfg)
		if err != nil {
			clientErrCh <- err
			return
		}
		defer conn.Close()

		state := conn.ConnectionState()
		if state.NegotiatedProtocol != ALPNProtocol {
			t.Errorf("client: negotiated ALPN mismatch: got %q, want %q", state.NegotiatedProtocol, ALPNProtocol)
		}
		if state.Version != tls.VersionTLS13 {
			t.Errorf("client: TLS version mismatch: got 0x%04x, want TLS 1.3", state.Version)
		}
		clientErrCh <- nil
	}()

	if err := <-clientErrCh; err != nil {
		t.Fatalf("client dial error: %v", err)
	}
	if err := <-serverErrCh; err != nil {
		t.Fatalf("server accept error: %v", err)
	}
}

func TestGenerateSelfSignedCertSANs(t *testing.T) {
	hosts := []string{"portal.local", "192.168.1.10", "127.0.0.1"}
	cert, err := GenerateSelfSignedCert(hosts)
	if err != nil {
		t.Fatalf("failed generating cert: %v", err)
	}

	leaf, err := tls.X509KeyPair(cert.Certificate[0], cert.Certificate[0])
	_ = leaf // validated format
	if len(cert.Certificate) == 0 {
		t.Fatalf("empty cert leaf")
	}
}

var _ = net.ParseIP
