package transport

import (
	"context"
	"fmt"
	"net"
	"time"
)

// SocketOptions captures low-level TCP tuning applied to tunnel transports.
// Zero values select production defaults via DefaultSocketOptions.
type SocketOptions struct {
	// NoDelay disables Nagle's algorithm for snappy small-frame delivery.
	NoDelay bool
	// KeepAlive enables TCP keepalive probes.
	KeepAlive bool
	// KeepAliveIdle is the idle time before the first probe.
	KeepAliveIdle time.Duration
	// KeepAliveInterval is the interval between probes (best effort; the
	// stdlib exposes idle time portably, interval via OS defaults).
	KeepAliveInterval time.Duration
	// ReadBufferSize sets SO_RCVBUF when positive.
	ReadBufferSize int
	// WriteBufferSize sets SO_SNDBUF when positive.
	WriteBufferSize int
}

// DefaultSocketOptions returns production-grade TCP tuning for tunnels:
// no Nagle delay, 30s keepalive idle, 256KB socket buffers.
func DefaultSocketOptions() SocketOptions {
	return SocketOptions{
		NoDelay:           true,
		KeepAlive:         true,
		KeepAliveIdle:     30 * time.Second,
		KeepAliveInterval: 10 * time.Second,
		ReadBufferSize:    256 * 1024,
		WriteBufferSize:   256 * 1024,
	}
}

// TuneConn applies SocketOptions to conn when it exposes *net.TCPConn.
// Non-TCP connections (TLS wrappers, pipes in tests) are left untouched and
// report nil so tuning never breaks alternative transports.
func TuneConn(conn net.Conn, opts SocketOptions) error {
	tcp, ok := conn.(*net.TCPConn)
	if !ok {
		return nil
	}
	if err := tcp.SetNoDelay(opts.NoDelay); err != nil {
		return fmt.Errorf("set TCP_NODELAY: %w", err)
	}
	if err := tcp.SetKeepAlive(opts.KeepAlive); err != nil {
		return fmt.Errorf("set keepalive: %w", err)
	}
	if opts.KeepAlive && opts.KeepAliveIdle > 0 {
		if err := tcp.SetKeepAlivePeriod(opts.KeepAliveIdle); err != nil {
			return fmt.Errorf("set keepalive period: %w", err)
		}
	}
	if opts.ReadBufferSize > 0 {
		if err := tcp.SetReadBuffer(opts.ReadBufferSize); err != nil {
			return fmt.Errorf("set read buffer: %w", err)
		}
	}
	if opts.WriteBufferSize > 0 {
		if err := tcp.SetWriteBuffer(opts.WriteBufferSize); err != nil {
			return fmt.Errorf("set write buffer: %w", err)
		}
	}
	return nil
}

// TuneListener wraps ln so every accepted connection is tuned with opts.
// Accept errors are passed through unchanged.
func TuneListener(ln net.Listener, opts SocketOptions) net.Listener {
	return &tunedListener{Listener: ln, opts: opts}
}

type tunedListener struct {
	net.Listener
	opts SocketOptions
}

// Accept tunes each connection before returning it. Tuning failures close the
// raw connection and are reported as accept errors.
func (l *tunedListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	if err := TuneConn(conn, l.opts); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

// DialTuned dials network/address with a timeout and applies SocketOptions to
// the resulting TCP connection.
func DialTuned(ctx context.Context, network, address string, timeout time.Duration, opts SocketOptions) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: opts.KeepAliveIdle}
	conn, err := dialer.DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	if err := TuneConn(conn, opts); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}
