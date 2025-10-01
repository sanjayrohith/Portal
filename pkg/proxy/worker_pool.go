package proxy

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"

	"github.com/sanjayrohith/portal/pkg/mux"
)

// WorkerPool manages a pool of concurrent workers dispatching incoming multiplexed streams
// to local upstream services without blocking the multiplexer receiver.
type WorkerPool struct {
	session     *mux.Session
	bridge      *StreamBridge
	maxWorkers  int
	streamQueue chan net.Conn
	activeTasks atomic.Int64
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.Mutex
	closed      bool
}

// NewWorkerPool creates a new worker pool for dispatching streams from a multiplexer session.
func NewWorkerPool(session *mux.Session, bridge *StreamBridge, maxWorkers int, queueSize int) *WorkerPool {
	if maxWorkers <= 0 {
		maxWorkers = 64
	}
	if queueSize <= 0 {
		queueSize = 256
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &WorkerPool{
		session:     session,
		bridge:      bridge,
		maxWorkers:  maxWorkers,
		streamQueue: make(chan net.Conn, queueSize),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start launches worker goroutines and starts consuming streams from the multiplexer session.
func (p *WorkerPool) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return
	}

	// Spawn worker goroutines
	for i := 0; i < p.maxWorkers; i++ {
		p.wg.Add(1)
		go p.workerLoop()
	}

	// Start listener pulling streams from session
	p.wg.Add(1)
	go p.acceptLoop()
}

func (p *WorkerPool) workerLoop() {
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case stream, ok := <-p.streamQueue:
			if !ok {
				return
			}
			p.activeTasks.Add(1)
			_ = p.bridge.BridgeStream(p.ctx, stream)
			p.activeTasks.Add(-1)
		}
	}
}

func (p *WorkerPool) acceptLoop() {
	defer p.wg.Done()

	for {
		// Asynchronously poll or accept stream
		acceptDone := make(chan struct{})
		var stream *mux.Stream
		var err error

		go func() {
			stream, err = p.session.AcceptStream()
			close(acceptDone)
		}()

		select {
		case <-p.ctx.Done():
			return
		case <-acceptDone:
			if err != nil {
				return
			}
		}

		select {
		case <-p.ctx.Done():
			_ = stream.Close()
			return
		case p.streamQueue <- stream:
		default:
			// Queue full: reject with 503 Service Unavailable to avoid deadlock
			go func(s net.Conn) {
				writeServiceUnavailable(s, errors.New("worker queue saturated"))
				_ = s.Close()
			}(stream)
		}
	}
}

// ActiveWorkers returns the count of currently processing tasks.
func (p *WorkerPool) ActiveWorkers() int64 {
	return p.activeTasks.Load()
}

// Stop gracefully shuts down the worker pool and closes accepted streams.
func (p *WorkerPool) Stop() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	p.cancel()
	p.mu.Unlock()

	p.wg.Wait()
}

func writeServiceUnavailable(s net.Conn, err error) {
	resp := "HTTP/1.1 503 Service Unavailable\r\n" +
		"Content-Type: text/plain\r\n" +
		"Content-Length: 29\r\n" +
		"Connection: close\r\n" +
		"\r\n" +
		"503 Service Pool Saturated\n"
	_, _ = s.Write([]byte(resp))
}
