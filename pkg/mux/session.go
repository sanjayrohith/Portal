package mux

import (
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	portalErr "github.com/sanjayrohith/portal/pkg/errors"
	"github.com/sanjayrohith/portal/pkg/protocol"
)

// Session manages concurrent logical streams multiplexed over a single underlying connection.
type Session struct {
	conn     net.Conn
	isServer bool
	writer   *protocol.Writer
	reader   *protocol.Reader

	streamsMu sync.RWMutex
	streams   map[uint32]*Stream

	acceptCh  chan *Stream
	closeCh   chan struct{}
	closeOnce sync.Once
	closed    atomic.Bool

	nextStreamID atomic.Uint32

	// Heartbeat and RTT manager
	pingMgr     *pingManager
	pongHandler func(nonce uint32)
	pongMu      sync.RWMutex
}

// NewSession creates and starts a new multiplexer Session over conn.
func NewSession(conn net.Conn, isServer bool) *Session {
	s := &Session{
		conn:     conn,
		isServer: isServer,
		writer:   protocol.NewWriter(conn),
		reader:   protocol.NewReader(conn),
		streams:  make(map[uint32]*Stream),
		acceptCh: make(chan *Stream, 256),
		closeCh:  make(chan struct{}),
	}

	if isServer {
		s.nextStreamID.Store(2) // Server initiates even stream IDs
	} else {
		s.nextStreamID.Store(1) // Client initiates odd stream IDs
	}

	s.pingMgr = newPingManager(s)

	go s.recvLoop()

	return s
}

// OpenStream initiates a new logical stream, sends an OPEN frame, and registers it.
func (s *Session) OpenStream() (*Stream, error) {
	if s.closed.Load() {
		return nil, portalErr.ErrConnectionClosed
	}

	streamID := s.nextStreamID.Add(2) - 2

	stream := newStream(streamID, s)

	s.streamsMu.Lock()
	s.streams[streamID] = stream
	s.streamsMu.Unlock()

	// Send OPEN frame
	openFrame := protocol.NewOpenFrame(streamID)
	if err := s.sendFrame(openFrame); err != nil {
		s.onStreamClosed(streamID)
		return nil, fmt.Errorf("failed to send OPEN frame: %w", err)
	}

	return stream, nil
}

// AcceptStream blocks until an incoming stream is initiated by the remote peer.
func (s *Session) AcceptStream() (*Stream, error) {
	select {
	case <-s.closeCh:
		return nil, io.EOF
	case stream, ok := <-s.acceptCh:
		if !ok {
			return nil, io.EOF
		}
		return stream, nil
	}
}

// Ping sends a heartbeat PING frame and measures round-trip time.
func (s *Session) Ping(timeout time.Duration) (time.Duration, error) {
	return s.pingMgr.Ping(timeout)
}

// LastRTT returns the latest measured heartbeat round-trip latency.
func (s *Session) LastRTT() time.Duration {
	return s.pingMgr.LastRTT()
}

// StartKeepalive enables periodic background heartbeat checks.
func (s *Session) StartKeepalive(interval time.Duration, timeout time.Duration, maxConsecutiveFailures int) {
	s.pingMgr.StartKeepalive(interval, timeout, maxConsecutiveFailures)
}

// sendFrame transmits a binary frame to the underlying connection.
func (s *Session) sendFrame(f *protocol.Frame) error {
	if s.closed.Load() {
		return portalErr.ErrConnectionClosed
	}
	return s.writer.WriteFrame(f)
}

// onStreamClosed unregisters a closed stream from the session.
func (s *Session) onStreamClosed(id uint32) {
	s.streamsMu.Lock()
	delete(s.streams, id)
	s.streamsMu.Unlock()
}

func (s *Session) localAddr() net.Addr {
	return s.conn.LocalAddr()
}

func (s *Session) remoteAddr() net.Addr {
	return s.conn.RemoteAddr()
}

// Close gracefully terminates the session and all active streams.
func (s *Session) Close() error {
	var err error
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		close(s.closeCh)

		// Drop the PONG dispatch reference so the closed session no longer
		// pins the heartbeat manager.
		s.pongMu.Lock()
		s.pongHandler = nil
		s.pongMu.Unlock()

		// Close underlying network transport
		err = s.conn.Close()

		// Close all active streams
		s.streamsMu.Lock()
		activeStreams := make([]*Stream, 0, len(s.streams))
		for _, st := range s.streams {
			activeStreams = append(activeStreams, st)
		}
		s.streams = make(map[uint32]*Stream)
		s.streamsMu.Unlock()

		for _, st := range activeStreams {
			_ = st.Reset()
		}
	})
	return err
}

// IsClosed reports whether the session has been terminated.
func (s *Session) IsClosed() bool {
	return s.closed.Load()
}

// ActiveStreamsCount returns the number of currently open multiplexed streams.
func (s *Session) ActiveStreamsCount() int {
	s.streamsMu.RLock()
	defer s.streamsMu.RUnlock()
	return len(s.streams)
}

// recvLoop continuously decodes binary frames from the underlying connection.
func (s *Session) recvLoop() {
	defer s.Close()

	for {
		frame, err := s.reader.ReadFrame()
		if err != nil {
			return
		}

		s.handleIncomingFrame(frame)
	}
}

func (s *Session) handleIncomingFrame(f *protocol.Frame) {
	switch f.Type {
	case protocol.FrameOpen:
		if s.closed.Load() {
			// Session is gone: refuse the stream immediately instead of
			// queueing state nobody will ever accept or clean up.
			rstFrame := protocol.NewCloseFrame(f.StreamID, protocol.FlagRst)
			_ = s.sendFrame(rstFrame)
			return
		}
		stream := newStream(f.StreamID, s)

		s.streamsMu.Lock()
		s.streams[f.StreamID] = stream
		s.streamsMu.Unlock()

		select {
		case s.acceptCh <- stream:
		default:
			// Queue full; reset incoming stream
			_ = stream.Reset()
		}

	case protocol.FrameData:
		s.streamsMu.RLock()
		stream, ok := s.streams[f.StreamID]
		s.streamsMu.RUnlock()

		if ok {
			fin := f.Flags.Has(protocol.FlagFin)
			_ = stream.pushData(f.Payload, fin)
		} else {
			// If stream does not exist, send RST
			rstFrame := protocol.NewCloseFrame(f.StreamID, protocol.FlagRst)
			_ = s.sendFrame(rstFrame)
		}

	case protocol.FrameWindowUpdate:
		s.streamsMu.RLock()
		stream, ok := s.streams[f.StreamID]
		s.streamsMu.RUnlock()

		if ok {
			credit, err := f.ParseWindowUpdateCredit()
			if err == nil {
				stream.handleWindowUpdate(credit)
			}
		}

	case protocol.FrameClose:
		s.streamsMu.RLock()
		stream, ok := s.streams[f.StreamID]
		s.streamsMu.RUnlock()

		if ok {
			if f.Flags.Has(protocol.FlagRst) {
				_ = stream.Reset()
			} else {
				_ = stream.pushData(nil, true)
			}
		}

	case protocol.FramePing:
		nonce, err := f.ParsePingNonce()
		if err == nil {
			pongFrame := protocol.NewPongFrame(nonce)
			_ = s.sendFrame(pongFrame)
		}

	case protocol.FramePong:
		nonce, err := f.ParsePingNonce()
		if err == nil {
			s.pongMu.RLock()
			handler := s.pongHandler
			s.pongMu.RUnlock()
			if handler != nil {
				handler(nonce)
			}
		}
	}
}
