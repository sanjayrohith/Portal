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
	httpServer *http.Server
	addr       string
}

// NewServer creates an inspector server on 127.0.0.1:4040.
func NewServer(buffer *RingBuffer) *Server {
	return newServer(DefaultInspectorAddr, buffer)
}

// NewServerWithAddr creates an inspector server on a custom loopback port.
// The address must use the 127.0.0.1 interface; non-loopback addresses are
// rejected before any socket is opened.
func NewServerWithAddr(addr string, buffer *RingBuffer) (*Server, error) {
	if err := validateInspectorAddr(addr); err != nil {
		return nil, err
	}
	return newServer(addr, buffer), nil
}

func newServer(addr string, buffer *RingBuffer) *Server {
	if buffer == nil {
		buffer = NewRingBuffer(0)
	}
	return &Server{
		addr: addr,
		httpServer: &http.Server{
			Handler: newAPIHandler(buffer),
		},
	}
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
	return net.Listen("tcp4", s.addr)
}

// Serve serves inspector requests on an already-opened listener.
func (s *Server) Serve(listener net.Listener) error {
	if listener == nil {
		return fmt.Errorf("inspector listener cannot be nil")
	}
	return s.httpServer.Serve(listener)
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

func newAPIHandler(*RingBuffer) http.Handler {
	return http.NotFoundHandler()
}
