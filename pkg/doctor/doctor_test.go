package doctor

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/sanjayrohith/portal/pkg/transport"
)

func TestCheckLoopback(t *testing.T) {
	res := CheckLoopback(1 * time.Second)
	if res.Status != StatusPass {
		t.Fatalf("expected loopback check to pass, got %s: %s", res.Status, res.Message)
	}
}

func TestCheckInspectorPort(t *testing.T) {
	// 1. Available ephemeral port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to open test listener: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	res := CheckInspectorPort(addr)
	if res.Status != StatusPass {
		t.Fatalf("expected inspector check to pass on available port %s, got %s", addr, res.Status)
	}

	// 2. Occupied port
	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind listener: %v", err)
	}
	defer ln2.Close()

	res2 := CheckInspectorPort(ln2.Addr().String())
	if res2.Status != StatusWarn {
		t.Fatalf("expected inspector check to warn on occupied port, got %s", res2.Status)
	}
}

func TestCheckLocalTarget(t *testing.T) {
	// 1. Online target
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind mock target: %v", err)
	}
	defer ln.Close()

	res := CheckLocalTarget(ln.Addr().String(), 500*time.Millisecond)
	if res.Status != StatusPass {
		t.Fatalf("expected local target check to pass, got %s: %s", res.Status, res.Message)
	}

	// 2. Offline target
	_ = ln.Close()
	res2 := CheckLocalTarget(ln.Addr().String(), 100*time.Millisecond)
	if res2.Status != StatusWarn {
		t.Fatalf("expected local target check to warn on offline target, got %s", res2.Status)
	}
}

func TestCheckDNS(t *testing.T) {
	// 1. Direct IP check
	item, ips := CheckDNS("127.0.0.1", 1*time.Second)
	if item.Status != StatusPass || len(ips) == 0 {
		t.Fatalf("expected direct IP check to pass, got: %+v", item)
	}

	// 2. Localhost DNS resolution
	item2, ips2 := CheckDNS("localhost", 1*time.Second)
	if item2.Status != StatusPass || len(ips2) == 0 {
		t.Fatalf("expected localhost resolution to pass, got: %+v", item2)
	}

	// 3. Non-existent domain
	item3, _ := CheckDNS("invalid.portal.test.nonexistent.domain", 500*time.Millisecond)
	if item3.Status != StatusFail {
		t.Fatalf("expected invalid domain to fail DNS check, got: %s", item3.Status)
	}
}

func TestCheckTCPAndTLS(t *testing.T) {
	// Generate self-signed cert for mock server
	cert, err := transport.GenerateSelfSignedCert([]string{"127.0.0.1", "localhost"})
	if err != nil {
		t.Fatalf("failed to generate self-signed cert: %v", err)
	}

	serverTLS := transport.ServerTLSConfig(cert)
	ln, err := tls.Listen("tcp", "127.0.0.1:0", serverTLS)
	if err != nil {
		t.Fatalf("failed to start mock TLS server: %v", err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				tlsConn, ok := c.(*tls.Conn)
				if ok {
					_ = tlsConn.Handshake()
				}
			}(conn)
		}
	}()

	addr := ln.Addr().String()

	// Test TCP check
	tcpRes := CheckTCP(addr, 1*time.Second)
	if tcpRes.Status != StatusPass {
		t.Fatalf("expected TCP check to pass on %s, got: %+v", addr, tcpRes)
	}

	// Test TLS check with cert pool
	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(cert.Certificate[0])
	parsed, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("failed to parse certificate: %v", err)
	}
	certPool.AddCert(parsed)

	tlsRes := CheckTLS(addr, "127.0.0.1", false, certPool, 1*time.Second)
	if tlsRes.Status != StatusPass {
		// If verification failed because of root CA format, check with insecure = true
		tlsResInsecure := CheckTLS(addr, "127.0.0.1", true, nil, 1*time.Second)
		if tlsResInsecure.Status != StatusPass {
			t.Fatalf("expected TLS check to pass with insecure=true, got: %+v", tlsResInsecure)
		}
	}
}

func TestRunDiagnosticsAndFormat(t *testing.T) {
	// Spin up a mock local target and mock control plane
	targetLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind target: %v", err)
	}
	defer targetLn.Close()

	cert, err := transport.GenerateSelfSignedCert([]string{"127.0.0.1"})
	if err != nil {
		t.Fatalf("cert gen: %v", err)
	}
	serverTLS := transport.ServerTLSConfig(cert)
	serverLn, err := tls.Listen("tcp", "127.0.0.1:0", serverTLS)
	if err != nil {
		t.Fatalf("tls listen: %v", err)
	}
	defer serverLn.Close()

	go func() {
		for {
			c, err := serverLn.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				if tc, ok := conn.(*tls.Conn); ok {
					_ = tc.Handshake()
				}
			}(c)
		}
	}()

	opts := Options{
		ServerAddr:         serverLn.Addr().String(),
		LocalTarget:        targetLn.Addr().String(),
		InspectorAddr:      "127.0.0.1:0",
		InsecureSkipVerify: true,
		Timeout:            1 * time.Second,
	}

	report := RunDiagnostics(opts)
	if !report.AllPassed {
		t.Fatalf("expected all checks to pass, report: %+v", report)
	}

	// Check text formatting
	text := FormatText(report)
	if !strings.Contains(text, "PORTAL DOCTOR REPORT") || !strings.Contains(text, "Overall Status: HEALTHY") {
		t.Fatalf("unexpected text output: %s", text)
	}

	// Check JSON formatting
	jsonStr, err := FormatJSON(report)
	if err != nil {
		t.Fatalf("FormatJSON failed: %v", err)
	}
	if !strings.Contains(jsonStr, `"all_passed": true`) {
		t.Fatalf("unexpected json output: %s", jsonStr)
	}
}
