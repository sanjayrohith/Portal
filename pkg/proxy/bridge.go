package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// StreamBridge bridges an edge multiplexer stream and a local upstream target.
type StreamBridge struct {
	dialer   Dialer
	rewriter *HostRewriter
	timeout  time.Duration
}

// NewStreamBridge creates a new StreamBridge.
func NewStreamBridge(dialer Dialer, rewriter *HostRewriter, timeout time.Duration) *StreamBridge {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &StreamBridge{
		dialer:   dialer,
		rewriter: rewriter,
		timeout:  timeout,
	}
}

// BridgeStream handles an incoming stream from the edge multiplexer, dials the upstream,
// optionally rewrites HTTP headers, and pipes bidirectional traffic.
func (b *StreamBridge) BridgeStream(ctx context.Context, stream net.Conn) error {
	defer stream.Close()

	// Dial upstream service
	dialCtx, dialCancel := context.WithTimeout(ctx, 5*time.Second)
	defer dialCancel()

	upstream, err := b.dialer.Dial(dialCtx)
	if err != nil {
		// Upstream unreachable: write HTTP 502 Bad Gateway response to stream
		writeBadGateway(stream, err)
		return fmt.Errorf("bridge dial failed: %w", err)
	}
	defer upstream.Close()

	if b.rewriter != nil {
		return b.bridgeHTTPWithRewrite(stream, upstream)
	}

	return b.PipeBidirectional(stream, upstream)
}

// bridgeHTTPWithRewrite parses the request, rewrites headers, and forwards to upstream.
func (b *StreamBridge) bridgeHTTPWithRewrite(stream net.Conn, upstream net.Conn) error {
	req, err := ReadHTTPRequest(stream)
	if err != nil {
		return fmt.Errorf("failed to read HTTP request: %w", err)
	}

	if b.rewriter != nil {
		b.rewriter.RewriteRequest(req)
	}

	if err := WriteHTTPRequest(upstream, req); err != nil {
		return fmt.Errorf("failed to write request to upstream: %w", err)
	}

	// Once the request header/body is piped to upstream, pipe response back
	return b.PipeBidirectional(stream, upstream)
}

// PipeBidirectional establishes full-duplex streaming between two net.Conn endpoints.
func (b *StreamBridge) PipeBidirectional(c1, c2 net.Conn) error {
	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, err := io.Copy(c2, c1)
		// Close writing side if supported
		if cw, ok := c2.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		}
		errCh <- err
	}()

	go func() {
		defer wg.Done()
		_, err := io.Copy(c1, c2)
		if cw, ok := c1.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		}
		errCh <- err
	}()

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil && err != io.EOF {
			return err
		}
	}
	return nil
}

func writeBadGateway(w io.Writer, dialErr error) {
	body := fmt.Sprintf("502 Bad Gateway: failed to connect to upstream service: %v\n", dialErr)
	resp := fmt.Sprintf(
		"HTTP/1.1 502 Bad Gateway\r\n"+
			"Content-Type: text/plain; charset=utf-8\r\n"+
			"Content-Length: %d\r\n"+
			"Connection: close\r\n"+
			"\r\n"+
			"%s",
		len(body), body,
	)
	_, _ = io.WriteString(w, resp)
}
