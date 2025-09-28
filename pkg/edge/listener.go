package edge

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// ReverseProxyListener manages HTTP and HTTPS server listeners on the edge.
type ReverseProxyListener struct {
	httpAddr    string
	httpsAddr   string
	tlsConfig   *tls.Config
	handler     http.Handler
	httpServer  *http.Server
	httpsServer *http.Server
	mu          sync.Mutex
	closed      bool
}

// NewReverseProxyListener creates a new ReverseProxyListener.
func NewReverseProxyListener(httpAddr, httpsAddr string, tlsConfig *tls.Config, handler http.Handler) *ReverseProxyListener {
	l := &ReverseProxyListener{
		httpAddr:  httpAddr,
		httpsAddr: httpsAddr,
		tlsConfig: tlsConfig,
		handler:   handler,
	}

	if httpAddr != "" {
		l.httpServer = &http.Server{
			Addr:              httpAddr,
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       60 * time.Second,
		}
	}

	if httpsAddr != "" {
		l.httpsServer = &http.Server{
			Addr:              httpsAddr,
			Handler:           handler,
			TLSConfig:         tlsConfig,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       60 * time.Second,
		}
	}

	return l
}

// Start starts listening on HTTP and HTTPS addresses in background goroutines.
func (l *ReverseProxyListener) Start(errCh chan<- error) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return errors.New("listener already closed")
	}

	if l.httpServer != nil {
		ln, err := net.Listen("tcp", l.httpAddr)
		if err != nil {
			return fmt.Errorf("failed to listen on HTTP addr %s: %w", l.httpAddr, err)
		}
		go func() {
			if err := l.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				if errCh != nil {
					errCh <- fmt.Errorf("HTTP listener error: %w", err)
				}
			}
		}()
	}

	if l.httpsServer != nil {
		if l.tlsConfig == nil {
			return errors.New("TLS config must be provided for HTTPS listener")
		}
		ln, err := net.Listen("tcp", l.httpsAddr)
		if err != nil {
			return fmt.Errorf("failed to listen on HTTPS addr %s: %w", l.httpsAddr, err)
		}
		tlsLn := tls.NewListener(ln, l.tlsConfig)
		go func() {
			if err := l.httpsServer.Serve(tlsLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
				if errCh != nil {
					errCh <- fmt.Errorf("HTTPS listener error: %w", err)
				}
			}
		}()
	}

	return nil
}

// Shutdown gracefully stops both HTTP and HTTPS listeners.
func (l *ReverseProxyListener) Shutdown(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return nil
	}
	l.closed = true

	var errs []error
	if l.httpServer != nil {
		if err := l.httpServer.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("HTTP shutdown error: %w", err))
		}
	}
	if l.httpsServer != nil {
		if err := l.httpsServer.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("HTTPS shutdown error: %w", err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
