package inspector

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
)

const (
	// DefaultInspectorAddr is the loopback-only address used by the inspector.
	DefaultInspectorAddr = "127.0.0.1:4040"
)

// Server hosts the local request inspector. Its listener is intentionally
// restricted to IPv4 loopback so captured traffic cannot be exposed remotely.
type Server struct {
	httpServer   *http.Server
	addr         string
	replayTarget string
	buffer       *RingBuffer
	events       *eventHub
}

// NewServer creates an inspector server on 127.0.0.1:4040.
func NewServer(buffer *RingBuffer) *Server {
	return newServer(DefaultInspectorAddr, buffer, "")
}

// NewServerWithReplayTarget creates an inspector server that can replay
// captured requests to the supplied loopback target.
func NewServerWithReplayTarget(buffer *RingBuffer, target string) (*Server, error) {
	if err := validateReplayTarget(target); err != nil {
		return nil, err
	}
	return newServer(DefaultInspectorAddr, buffer, target), nil
}

// NewServerWithAddr creates an inspector server on a custom loopback port.
// The address must use the 127.0.0.1 interface; non-loopback addresses are
// rejected before any socket is opened.
func NewServerWithAddr(addr string, buffer *RingBuffer) (*Server, error) {
	if err := validateInspectorAddr(addr); err != nil {
		return nil, err
	}
	return newServer(addr, buffer, ""), nil
}

func newServer(addr string, buffer *RingBuffer, replayTarget string) *Server {
	if buffer == nil {
		buffer = NewRingBuffer(0)
	}
	events := newEventHub()
	return &Server{
		addr:         addr,
		replayTarget: replayTarget,
		buffer:       buffer,
		events:       events,
		httpServer: &http.Server{
			Handler: newAPIHandler(buffer, replayTarget, events),
		},
	}
}

// Publish stores a captured transaction and notifies connected inspector
// clients. It is the preferred entry point for new captured exchanges.
func (s *Server) Publish(transaction *CapturedTransaction) {
	if transaction == nil {
		return
	}
	s.buffer.Add(transaction)
	s.events.Publish(transaction)
}

// Addr returns the configured loopback address.
func (s *Server) Addr() string {
	return s.addr
}

// Handler returns the HTTP handler used by the inspector server.
func (s *Server) Handler() http.Handler {
	return s.httpServer.Handler
}

// Listen opens the loopback-only listener without starting the HTTP serving
// loop. This is useful for callers that need to manage Serve themselves.
func (s *Server) Listen() (net.Listener, error) {
	if err := validateInspectorAddr(s.addr); err != nil {
		return nil, err
	}
	return net.Listen("tcp4", s.addr)
}

// Serve serves inspector requests on an already-opened listener. The
// listener address is verified to be strict loopback first, so a caller
// cannot accidentally (or maliciously) expose captured traffic on an
// external interface by passing in a 0.0.0.0 listener.
func (s *Server) Serve(listener net.Listener) error {
	if listener == nil {
		return fmt.Errorf("inspector listener cannot be nil")
	}
	if err := verifyLoopbackListener(listener); err != nil {
		return err
	}
	return s.httpServer.Serve(listener)
}

// verifyLoopbackListener rejects listeners bound anywhere but 127.0.0.1.
func verifyLoopbackListener(listener net.Listener) error {
	addr := listener.Addr().String()
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("inspector listener has invalid address %q: %w", addr, err)
	}
	if host != "127.0.0.1" {
		return fmt.Errorf("inspector listener must bind strictly to loopback (127.0.0.1), got %q", host)
	}
	return nil
}

// ListenAndServe opens the configured loopback listener and serves requests.
func (s *Server) ListenAndServe() error {
	listener, err := s.Listen()
	if err != nil {
		return fmt.Errorf("listen on inspector address %s: %w", s.addr, err)
	}
	return s.Serve(listener)
}

// Shutdown gracefully stops the inspector HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func validateInspectorAddr(addr string) error {
	addr = strings.TrimSpace(addr)
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid inspector address %q: %w", addr, err)
	}
	if host != "127.0.0.1" {
		return fmt.Errorf("inspector address must bind strictly to loopback (127.0.0.1), got %q", host)
	}
	if port == "" {
		return fmt.Errorf("inspector address must include a port")
	}
	return nil
}
