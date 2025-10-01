package mux

import (
	"io"
	"net"
	"sync"
	"testing"
)

func TestStreamHalfCloseAndCleanTeardown(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	clientSession := NewSession(c1, false)
	serverSession := NewSession(c2, true)
	defer clientSession.Close()
	defer serverSession.Close()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		serverStream, err := serverSession.AcceptStream()
		if err != nil {
			t.Errorf("server accept stream failed: %v", err)
			return
		}
		defer serverStream.Close()

		// Read request until EOF (half-closed by client via CloseWrite / FIN)
		reqData, err := io.ReadAll(serverStream)
		if err != nil {
			t.Errorf("server read error: %v", err)
			return
		}
		if string(reqData) != "client request data" {
			t.Errorf("unexpected request data: %q", string(reqData))
		}

		// Write response back to client
		_, err = serverStream.Write([]byte("server response data"))
		if err != nil {
			t.Errorf("server write error: %v", err)
			return
		}
	}()

	clientStream, err := clientSession.OpenStream()
	if err != nil {
		t.Fatalf("client open stream failed: %v", err)
	}
	defer clientStream.Close()

	// Client writes request payload
	_, err = clientStream.Write([]byte("client request data"))
	if err != nil {
		t.Fatalf("client write failed: %v", err)
	}

	// Local half-close: CloseWrite sends FIN but leaves incoming reading half open
	if err := clientStream.CloseWrite(); err != nil {
		t.Fatalf("CloseWrite failed: %v", err)
	}

	if clientStream.State() != StreamHalfClosedLocal {
		t.Errorf("expected state HALF_CLOSED_LOCAL, got %s", clientStream.State())
	}

	// Client reads response from server
	respData, err := io.ReadAll(clientStream)
	if err != nil {
		t.Fatalf("client read response failed: %v", err)
	}

	if string(respData) != "server response data" {
		t.Errorf("expected 'server response data', got %q", string(respData))
	}

	wg.Wait()
}
