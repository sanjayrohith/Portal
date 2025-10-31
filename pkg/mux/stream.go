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
	flow   *flowController

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
		flow:    newFlowController(id, DefaultInitialWindowSize, sender),
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
			if n > 0 && s.flow != nil {
				// Replenish receive window asynchronously or immediately
				_ = s.flow.notifyConsumed(n)
			}
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

// Write sends payload bytes as DATA frames respecting windowed flow control.
func (s *Stream) Write(p []byte) (n int, err error) {
	currentState := StreamState(s.state.Load())
	if currentState == StreamClosed || currentState == StreamReset || currentState == StreamHalfClosedLocal {
		return 0, portalErr.ErrConnectionClosed
	}

	total := len(p)
	remaining := p

	for len(remaining) > 0 {
		currentState = StreamState(s.state.Load())
		if currentState == StreamClosed || currentState == StreamReset || currentState == StreamHalfClosedLocal {
			return n, portalErr.ErrConnectionClosed
		}

		targetChunk := len(remaining)
		if targetChunk > protocol.DefaultMaxPayloadLength {
			targetChunk = protocol.DefaultMaxPayloadLength
		}

		// Acquire flow control credit
		credit, err := s.flow.acquireSendCredit(targetChunk, s.closeCh)
		if err != nil {
			return n, err
		}
		if credit == 0 {
			return n, portalErr.ErrConnectionClosed
		}

		chunk := remaining[:credit]
		frame := protocol.NewDataFrame(s.id, protocol.FlagNone, chunk)

		if err := s.sender.sendFrame(frame); err != nil {
			return n, err
		}

		n += credit
		remaining = remaining[credit:]
	}

	return total, nil
}

// CloseWrite sends a FIN frame to shut down the local writing half of the stream,
// transitioning the stream to StreamHalfClosedLocal while leaving reading open.
func (s *Stream) CloseWrite() error {
	var err error
	s.closeOnce.Do(func() {
		close(s.closeCh)
		s.transitionCloseLocal()

		// Wake up any blocked writers in flow control
		if s.flow != nil {
			s.flow.sendMu.Lock()
			s.flow.sendCond.Broadcast()
			s.flow.sendMu.Unlock()
		}

		// Send CLOSE frame with FIN flag
		finFrame := protocol.NewCloseFrame(s.id, protocol.FlagFin)
		err = s.sender.sendFrame(finFrame)

		// Note: we do NOT unregister from session yet if the remote side hasn't closed,
		// allowing incoming data from remote to continue being received.
		if StreamState(s.state.Load()) == StreamClosed {
			s.sender.onStreamClosed(s.id)
		}
	})
	return err
}

// Close gracefully closes the stream in both directions.
func (s *Stream) Close() error {
	_ = s.CloseWrite()

	s.readMu.Lock()
	s.state.Store(uint32(StreamClosed))
	// Drop the read backing array so closed streams under sustained
	// open/close cycling do not pin buffers while awaiting GC.
	s.readBuf.Reset()
	s.readCond.Broadcast()
	s.readMu.Unlock()

	s.sender.onStreamClosed(s.id)
	return nil
}

// Reset aborts the stream immediately (sends RST).
func (s *Stream) Reset() error {
	s.state.Store(uint32(StreamReset))

	if s.flow != nil {
		s.flow.sendMu.Lock()
		s.flow.sendCond.Broadcast()
		s.flow.sendMu.Unlock()
	}

	rstFrame := protocol.NewCloseFrame(s.id, protocol.FlagRst)
	_ = s.sender.sendFrame(rstFrame)

	s.sender.onStreamClosed(s.id)

	s.readMu.Lock()
	s.readBuf.Reset()
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
				if nxt == StreamClosed {
					s.sender.onStreamClosed(s.id)
				}
				break
			}
		}
	}

	s.readCond.Broadcast()
	return nil
}

// handleWindowUpdate processes incoming WINDOW_UPDATE frames replenishing outbound send credit.
func (s *Stream) handleWindowUpdate(delta uint32) {
	if s.flow != nil {
		s.flow.addSendCredit(delta)
	}
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
