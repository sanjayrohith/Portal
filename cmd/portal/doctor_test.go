package main

import (
	"bytes"
	"crypto/tls"
	"net"
	"strings"
	"testing"

	"github.com/sanjayrohith/portal/pkg/transport"
)

func TestPortalDoctorCommand(t *testing.T) {
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

	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetErr(&output)
	rootCmd.SetArgs([]string{
		"doctor",
		"--server", serverLn.Addr().String(),
		"--target", targetLn.Addr().String(),
		"--inspector-addr", "127.0.0.1:0",
		"--insecure",
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("doctor command failed: %v\nOutput: %s", err, output.String())
	}

	outStr := output.String()
	if !strings.Contains(outStr, "PORTAL DOCTOR REPORT") {
		t.Fatalf("expected report header in output: %s", outStr)
	}
	if !strings.Contains(outStr, "Overall Status: HEALTHY") {
		t.Fatalf("expected healthy status in output: %s", outStr)
	}
}

func TestPortalDoctorCommandJSON(t *testing.T) {
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetErr(&output)
	rootCmd.SetArgs([]string{
		"doctor",
		"--server", "127.0.0.1:0",
		"--json",
		"--timeout", "100ms",
	})

	// Expected to exit with error because 127.0.0.1:0 won't accept connections, but JSON is still printed
	_ = rootCmd.Execute()

	outStr := output.String()
	if !strings.Contains(outStr, `"all_passed"`) {
		t.Fatalf("expected JSON output containing 'all_passed', got: %s", outStr)
	}
}
