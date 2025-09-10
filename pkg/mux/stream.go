package mux

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	portalErr "github.com/sanjayrohith/portal/pkg/errors"
	"github.com/sanjayrohith/portal/pkg/protocol"
)

// StreamState represents the lifecycle state of a multiplexed stream.
type StreamState uint32

const (
	StreamInit StreamState = iota
	StreamOpen
	StreamHalfClosedLocal
	StreamHalfClosedRemote
	StreamClosed
	StreamReset
)

func (s StreamState) String() string {
	switch s {
	case StreamInit:
		return "INIT"
	case StreamOpen:
		return "OPEN"
	case StreamHalfClosedLocal:
		return "HALF_CLOSED_LOCAL"
	case StreamHalfClosedRemote:
		return "HALF_CLOSED_REMOTE"
	case StreamClosed:
		return "CLOSED"
	case StreamReset:
		return "RESET"
	default:
		return fmt.Sprintf("StreamState(%d)", s)
	}
}

// frameSender abstracts writing frames back to the multiplexer session.
type frameSender interface {
	sendFrame(f *protocol.Frame) error
	onStreamClosed(id uint32)
	localAddr() net.Addr
	remoteAddr() net.Addr
}

// Stream represents an individual bidirectional multiplexed stream over a session.
type Stream struct {
	id     uint32
	sender frameSender

	state     atomic.Uint32
	closeOnce sync.Once
	closeCh   chan struct{}

	// Read buffer state
	readMu   sync.Mutex
	readCond *sync.Cond
	readBuf  bytes.Buffer

	// Deadlines
	readDeadline  time.Time
	writeDeadline time.Time
	deadlineMu    sync.RWMutex
}

// newStream initializes a logical Stream.
func newStream(id uint32, sender frameSender) *Stream {
	s := &Stream{
		id:      id,
		sender:  sender,
		closeCh: make(chan struct{}),
	}
	s.readCond = sync.NewCond(&s.readMu)
	s.state.Store(uint32(StreamOpen))
	return s
}

// ID returns the unique 32-bit stream identifier.
func (s *Stream) ID() uint32 {
	return s.id
}

// State returns the current lifecycle state of the stream.
func (s *Stream) State() StreamState {
	return StreamState(s.state.Load())
}

// Read reads received payload data from the stream buffer into p.
func (s *Stream) Read(p []byte) (n int, err error) {
	s.readMu.Lock()
	defer s.readMu.Unlock()

	for {
		currentState := StreamState(s.state.Load())
		if s.readBuf.Len() > 0 {
			n, err = s.readBuf.Read(p)
			return n, err
		}

		if currentState == StreamClosed || currentState == StreamReset {
			return 0, io.EOF
		}
		if currentState == StreamHalfClosedRemote {
			return 0, io.EOF
		}

		s.deadlineMu.RLock()
		deadline := s.readDeadline
		s.deadlineMu.RUnlock()

		if !deadline.IsZero() && !time.Now().Before(deadline) {
			return 0, fmt.Errorf("read deadline exceeded")
		}

		// Wait for more data or state change
		s.readCond.Wait()
	}
}

// Write sends payload bytes as DATA frames over the multiplexed transport.
func (s *Stream) Write(p []byte) (n int, err error) {
	currentState := StreamState(s.state.Load())
	if currentState == StreamClosed || currentState == StreamReset || currentState == StreamHalfClosedLocal {
		return 0, portalErr.ErrConnectionClosed
	}

	total := len(p)
	remaining := p

	for len(remaining) > 0 {
		chunkSize := len(remaining)
		if chunkSize > protocol.DefaultMaxPayloadLength {
			chunkSize = protocol.DefaultMaxPayloadLength
		}

		chunk := remaining[:chunkSize]
		frame := protocol.NewDataFrame(s.id, protocol.FlagNone, chunk)

		if err := s.sender.sendFrame(frame); err != nil {
			return n, err
		}

		n += chunkSize
		remaining = remaining[chunkSize:]
	}

	return total, nil
}

// Close gracefully closes the local writing half of the stream (sends FIN).
func (s *Stream) Close() error {
	var err error
	s.closeOnce.Do(func() {
		close(s.closeCh)
		s.transitionCloseLocal()

		// Send CLOSE frame with FIN flag
		finFrame := protocol.NewCloseFrame(s.id, protocol.FlagFin)
		_ = s.sender.sendFrame(finFrame)

		s.sender.onStreamClosed(s.id)

		s.readMu.Lock()
		s.readCond.Broadcast()
		s.readMu.Unlock()
	})
	return err
}

// Reset aborts the stream immediately (sends RST).
func (s *Stream) Reset() error {
	s.state.Store(uint32(StreamReset))
	rstFrame := protocol.NewCloseFrame(s.id, protocol.FlagRst)
	_ = s.sender.sendFrame(rstFrame)

	s.sender.onStreamClosed(s.id)

	s.readMu.Lock()
	s.readCond.Broadcast()
	s.readMu.Unlock()
	return nil
}

func (s *Stream) transitionCloseLocal() {
	for {
		current := StreamState(s.state.Load())
		if current == StreamClosed || current == StreamReset {
			return
		}
		var next StreamState
		if current == StreamHalfClosedRemote {
			next = StreamClosed
		} else {
			next = StreamHalfClosedLocal
		}
		if s.state.CompareAndSwap(uint32(current), uint32(next)) {
			return
		}
	}
}

// pushData feeds incoming network data frames into the stream read buffer.
func (s *Stream) pushData(data []byte, fin bool) error {
	s.readMu.Lock()
	defer s.readMu.Unlock()

	currentState := StreamState(s.state.Load())
	if currentState == StreamClosed || currentState == StreamReset {
		return errors.New("stream closed")
	}

	if len(data) > 0 {
		s.readBuf.Write(data)
	}

	if fin {
		for {
			cur := StreamState(s.state.Load())
			var nxt StreamState
			if cur == StreamHalfClosedLocal {
				nxt = StreamClosed
			} else {
				nxt = StreamHalfClosedRemote
			}
			if s.state.CompareAndSwap(uint32(cur), uint32(nxt)) {
				break
			}
		}
	}

	s.readCond.Broadcast()
	return nil
}

// LocalAddr returns the local network address of the underlying session.
func (s *Stream) LocalAddr() net.Addr {
	return s.sender.localAddr()
}

// RemoteAddr returns the remote network address of the underlying session.
func (s *Stream) RemoteAddr() net.Addr {
	return s.sender.remoteAddr()
}

// SetDeadline sets both read and write deadlines.
func (s *Stream) SetDeadline(t time.Time) error {
	s.deadlineMu.Lock()
	s.readDeadline = t
	s.writeDeadline = t
	s.deadlineMu.Unlock()

	s.readMu.Lock()
	s.readCond.Broadcast()
	s.readMu.Unlock()
	return nil
}

// SetReadDeadline sets the read deadline.
func (s *Stream) SetReadDeadline(t time.Time) error {
	s.deadlineMu.Lock()
	s.readDeadline = t
	s.deadlineMu.Unlock()

	s.readMu.Lock()
	s.readCond.Broadcast()
	s.readMu.Unlock()
	return nil
}

// SetWriteDeadline sets the write deadline.
func (s *Stream) SetWriteDeadline(t time.Time) error {
	s.deadlineMu.Lock()
	s.writeDeadline = t
	s.deadlineMu.Unlock()
	return nil
}
