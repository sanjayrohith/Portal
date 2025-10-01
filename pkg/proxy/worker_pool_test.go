package proxy

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/sanjayrohith/portal/pkg/mux"
)

func TestWorkerPool_ParallelDispatch(t *testing.T) {
	// Start mock local upstream server
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("worker response"))
	}))
	defer upstreamServer.Close()

	dialer := NewLocalDialer(upstreamServer.Listener.Addr().String(), 1*time.Second)
	bridge := NewStreamBridge(dialer, nil, 2*time.Second)

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	serverSession := mux.NewSession(serverConn, true)
	clientSession := mux.NewSession(clientConn, false)
	defer serverSession.Close()
	defer clientSession.Close()

	pool := NewWorkerPool(clientSession, bridge, 8, 32)
	pool.Start()
	defer pool.Stop()

	// Dispatch 10 parallel requests
	var wg sync.WaitGroup
	reqCount := 10
	wg.Add(reqCount)

	for i := 0; i < reqCount; i++ {
		go func() {
			defer wg.Done()
			stream, err := serverSession.OpenStream()
			if err != nil {
				t.Errorf("failed to open stream: %v", err)
				return
			}
			defer stream.Close()

			req, _ := http.NewRequest("GET", "/test", nil)
			if err := WriteHTTPRequest(stream, req); err != nil {
				t.Errorf("failed to write request: %v", err)
				return
			}

			resp, err := ReadHTTPResponse(stream, req)
			if err != nil {
				t.Errorf("failed to read response: %v", err)
				return
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			if !bytes.Equal(body, []byte("worker response")) {
				t.Errorf("unexpected body: %s", string(body))
			}
		}()
	}

	wg.Wait()
}
