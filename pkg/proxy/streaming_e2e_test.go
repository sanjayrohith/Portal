package proxy

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sanjayrohith/portal/pkg/mux"
)

func TestEndToEndChunkedResponseStreamsBeforeCompletion(t *testing.T) {
	firstChunkWritten := make(chan struct{})
	releaseSecondChunk := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("upstream response writer does not support flushing")
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "first\n")
		flusher.Flush()
		close(firstChunkWritten)
		<-releaseSecondChunk
		_, _ = io.WriteString(w, "second\n")
		flusher.Flush()
	}))
	defer upstream.Close()

	edgeConn, clientConn := net.Pipe()
	edgeSession := mux.NewSession(edgeConn, true)
	clientSession := mux.NewSession(clientConn, false)
	defer edgeSession.Close()
	defer clientSession.Close()

	bridge := NewStreamBridge(NewLocalDialer(upstream.Listener.Addr().String(), 2*time.Second), nil, 2*time.Second)
	bridgeDone := make(chan struct{})
	go func() {
		defer close(bridgeDone)
		stream, err := clientSession.AcceptStream()
		if err != nil {
			return
		}
		_ = bridge.BridgeStream(context.Background(), stream)
	}()

	stream, err := edgeSession.OpenStream()
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	request, err := http.NewRequest(http.MethodGet, "http://stream.portal.dev/events", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	request.Host = "stream.portal.dev"
	if err := WriteHTTPRequest(stream, request); err != nil {
		t.Fatalf("write request: %v", err)
	}

	select {
	case <-firstChunkWritten:
	case <-time.After(time.Second):
		t.Fatal("upstream did not flush the first response chunk")
	}

	response, err := http.ReadResponse(bufio.NewReader(stream), request)
	if err != nil {
		t.Fatalf("read response headers: %v", err)
	}
	defer response.Body.Close()
	if len(response.TransferEncoding) != 1 || response.TransferEncoding[0] != "chunked" {
		t.Fatalf("expected chunked transfer encoding, got %#v", response.TransferEncoding)
	}

	firstRead := make(chan struct {
		body []byte
		err  error
	}, 1)
	go func() {
		body := make([]byte, len("first\n"))
		_, err := io.ReadFull(response.Body, body)
		firstRead <- struct {
			body []byte
			err  error
		}{body: body, err: err}
	}()

	select {
	case result := <-firstRead:
		if result.err != nil {
			t.Fatalf("read first response chunk: %v", result.err)
		}
		if string(result.body) != "first\n" {
			t.Fatalf("first response chunk = %q, want %q", result.body, "first\n")
		}
	case <-time.After(time.Second):
		t.Fatal("first response chunk was not forwarded before response completion")
	}

	close(releaseSecondChunk)
	remaining, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read remaining response body: %v", err)
	}
	if string(remaining) != "second\n" {
		t.Fatalf("remaining response body = %q, want %q", remaining, "second\n")
	}

	_ = stream.Close()
	select {
	case <-bridgeDone:
	case <-time.After(time.Second):
		t.Fatal("stream bridge did not terminate after client close")
	}
}
