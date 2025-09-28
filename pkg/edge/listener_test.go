package edge

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/sanjayrohith/portal/pkg/transport"
)

func TestReverseProxyListener_Lifecycle(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	cert, err := transport.GenerateSelfSignedCert([]string{"localhost", "127.0.0.1"})
	if err != nil {
		t.Fatalf("failed to generate cert: %v", err)
	}

	serverTLS := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	listener := NewReverseProxyListener("127.0.0.1:28080", "127.0.0.1:28443", serverTLS, handler)
	errCh := make(chan error, 2)
	if err := listener.Start(errCh); err != nil {
		t.Fatalf("listener Start failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Test plain HTTP
	resp, err := http.Get("http://127.0.0.1:28080/")
	if err != nil {
		t.Fatalf("HTTP GET failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Errorf("expected ok, got %s", string(body))
	}

	// Test HTTPS with insecure skip verify for self-signed cert
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	httpsResp, err := client.Get("https://127.0.0.1:28443/")
	if err != nil {
		t.Fatalf("HTTPS GET failed: %v", err)
	}
	defer httpsResp.Body.Close()
	httpsBody, _ := io.ReadAll(httpsResp.Body)
	if string(httpsBody) != "ok" {
		t.Errorf("expected ok, got %s", string(httpsBody))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := listener.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}
