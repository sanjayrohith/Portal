package proxy

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"
)

// Dialer defines the interface for dialing an upstream address.
type Dialer interface {
	Dial(ctx context.Context) (net.Conn, error)
}

// LocalDialer dials a local target address (e.g. 127.0.0.1:3000) over TCP.
type LocalDialer struct {
	targetAddr string
	timeout    time.Duration
	netDialer  *net.Dialer
}

// NewLocalDialer creates a new LocalDialer for the specified target address.
func NewLocalDialer(targetAddr string, timeout time.Duration) *LocalDialer {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &LocalDialer{
		targetAddr: targetAddr,
		timeout:    timeout,
		netDialer: &net.Dialer{
			Timeout:   timeout,
			KeepAlive: 30 * time.Second,
		},
	}
}

// Target returns the configured upstream target address.
func (d *LocalDialer) Target() string {
	return d.targetAddr
}

// Dial establishes a TCP connection to the upstream target address.
func (d *LocalDialer) Dial(ctx context.Context) (net.Conn, error) {
	conn, err := d.netDialer.DialContext(ctx, "tcp", d.targetAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to local upstream at %s: %w", d.targetAddr, err)
	}
	return conn, nil
}

// ConnectionPool manages reusable connections to an upstream service.
type ConnectionPool struct {
	dialer   Dialer
	maxConns int
	conns    chan net.Conn
	mu       sync.Mutex
	closed   bool
}

// NewConnectionPool creates a pool of up to maxConns for the given dialer.
func NewConnectionPool(dialer Dialer, maxConns int) *ConnectionPool {
	if maxConns <= 0 {
		maxConns = 10
	}
	return &ConnectionPool{
		dialer:   dialer,
		maxConns: maxConns,
		conns:    make(chan net.Conn, maxConns),
	}
}

// Acquire gets a connection from the pool or dials a new one.
func (p *ConnectionPool) Acquire(ctx context.Context) (net.Conn, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, fmt.Errorf("connection pool is closed")
	}
	p.mu.Unlock()

	select {
	case conn := <-p.conns:
		return conn, nil
	default:
		return p.dialer.Dial(ctx)
	}
}

// Release returns a healthy connection back to the pool or closes it if the pool is full.
func (p *ConnectionPool) Release(conn net.Conn) {
	if conn == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		_ = conn.Close()
		return
	}

	select {
	case p.conns <- conn:
	default:
		_ = conn.Close()
	}
}

// Close closes all pooled connections and shuts down the pool.
func (p *ConnectionPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}
	p.closed = true
	close(p.conns)

	for conn := range p.conns {
		_ = conn.Close()
	}
	return nil
}
