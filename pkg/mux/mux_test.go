package mux

import (
	"bytes"
	"crypto/rand"
	"io"
	"net"
	"sync"
	"testing"
)

func TestConcurrentStreamsBidirectional(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	clientSession := NewSession(clientConn, false)
	serverSession := NewSession(serverConn, true)
	defer clientSession.Close()
	defer serverSession.Close()

	const numStreams = 25
	const payloadSize = 8192

	var serverWg sync.WaitGroup
	serverWg.Add(numStreams)

	// Server echo worker loop
	go func() {
		for i := 0; i < numStreams; i++ {
			stream, err := serverSession.AcceptStream()
			if err != nil {
				t.Errorf("server AcceptStream error: %v", err)
				return
			}
			go func(s *Stream) {
				defer serverWg.Done()
				defer s.Close()

				buf := make([]byte, payloadSize)
				n, err := io.ReadFull(s, buf)
				if err != nil {
					t.Errorf("server stream read error: %v", err)
					return
				}
				if _, err := s.Write(buf[:n]); err != nil {
					t.Errorf("server stream write error: %v", err)
					return
				}
			}(stream)
		}
	}()

	// Client concurrent stream dispatchers
	var clientWg sync.WaitGroup
	clientWg.Add(numStreams)

	for i := 0; i < numStreams; i++ {
		go func(streamIdx int) {
			defer clientWg.Done()

			stream, err := clientSession.OpenStream()
			if err != nil {
				t.Errorf("client OpenStream error: %v", err)
				return
			}
			defer stream.Close()

			payload := make([]byte, payloadSize)
			if _, err := rand.Read(payload); err != nil {
				t.Errorf("rand.Read error: %v", err)
				return
			}

			if _, err := stream.Write(payload); err != nil {
				t.Errorf("client stream write error: %v", err)
				return
			}

			echoed := make([]byte, payloadSize)
			if _, err := io.ReadFull(stream, echoed); err != nil {
				t.Errorf("client stream read echoed error: %v", err)
				return
			}

			if !bytes.Equal(payload, echoed) {
				t.Errorf("stream %d: echoed payload mismatch", streamIdx)
			}
		}(i)
	}

	clientWg.Wait()
	serverWg.Wait()
}

func TestStreamIsolationAndReset(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	clientSession := NewSession(clientConn, false)
	serverSession := NewSession(serverConn, true)
	defer clientSession.Close()
	defer serverSession.Close()

	var serverAccepted sync.WaitGroup
	serverAccepted.Add(2)

	var streamA_Server, streamB_Server *Stream
	go func() {
		var err error
		streamA_Server, err = serverSession.AcceptStream()
		if err != nil {
			t.Errorf("accept stream A error: %v", err)
		}
		serverAccepted.Done()

		streamB_Server, err = serverSession.AcceptStream()
		if err != nil {
			t.Errorf("accept stream B error: %v", err)
		}
		serverAccepted.Done()
	}()

	streamA_Client, err := clientSession.OpenStream()
	if err != nil {
		t.Fatalf("open stream A error: %v", err)
	}
	streamB_Client, err := clientSession.OpenStream()
	if err != nil {
		t.Fatalf("open stream B error: %v", err)
	}

	serverAccepted.Wait()

	// Abort stream A immediately
	if err := streamA_Client.Reset(); err != nil {
		t.Fatalf("reset stream A error: %v", err)
	}

	// Verify stream A server reads error/EOF
	readBuf := make([]byte, 100)
	_, readErr := streamA_Server.Read(readBuf)
	if readErr == nil {
		t.Errorf("expected error on reset stream A read, got nil")
	}

	// Verify Stream B remains fully functional
	testPayload := []byte("hello from isolated stream B")
	go func() {
		_, _ = streamB_Server.Write(testPayload)
		_ = streamB_Server.Close()
	}()

	bRecv := make([]byte, len(testPayload))
	if _, err := io.ReadFull(streamB_Client, bRecv); err != nil {
		t.Fatalf("stream B read error despite isolation: %v", err)
	}

	if !bytes.Equal(testPayload, bRecv) {
		t.Errorf("stream B payload mismatch: got %q, want %q", bRecv, testPayload)
	}
}

func TestBackpressureAndLargeTransfer(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	clientSession := NewSession(clientConn, false)
	serverSession := NewSession(serverConn, true)
	defer clientSession.Close()
	defer serverSession.Close()

	// 512 KB transfer (greater than DefaultInitialWindowSize 256 KB)
	const dataSize = 512 * 1024
	outboundData := make([]byte, dataSize)
	if _, err := rand.Read(outboundData); err != nil {
		t.Fatalf("rand.Read error: %v", err)
	}

	var serverDone sync.WaitGroup
	serverDone.Add(1)

	var receivedBuffer bytes.Buffer

	// Server accepts stream and reads slowly in small chunks with slight delays
	go func() {
		defer serverDone.Done()

		serverStream, err := serverSession.AcceptStream()
		if err != nil {
			t.Errorf("server AcceptStream error: %v", err)
			return
		}
		defer serverStream.Close()

		chunk := make([]byte, 16*1024)
		for {
			n, rErr := serverStream.Read(chunk)
			if n > 0 {
				receivedBuffer.Write(chunk[:n])
			}
			if rErr == io.EOF {
				break
			}
			if rErr != nil {
				t.Errorf("server read error: %v", rErr)
				return
			}
		}
	}()

	clientStream, err := clientSession.OpenStream()
	if err != nil {
		t.Fatalf("client OpenStream error: %v", err)
	}

	// Write entire 512KB payload
	go func() {
		_, wErr := clientStream.Write(outboundData)
		if wErr != nil {
			t.Errorf("client stream write error: %v", wErr)
		}
		_ = clientStream.Close()
	}()

	serverDone.Wait()

	if receivedBuffer.Len() != dataSize {
		t.Fatalf("received size mismatch: got %d, want %d", receivedBuffer.Len(), dataSize)
	}
	if !bytes.Equal(outboundData, receivedBuffer.Bytes()) {
		t.Fatalf("large transfer data corrupted during flow control windowing")
	}
}

func TestGracefulSessionTeardown(t *testing.T) {
	clientConn, serverConn := net.Pipe()

	clientSession := NewSession(clientConn, false)
	serverSession := NewSession(serverConn, true)

	stream, err := clientSession.OpenStream()
	if err != nil {
		t.Fatalf("OpenStream error: %v", err)
	}

	if clientSession.ActiveStreamsCount() != 1 {
		t.Errorf("expected 1 active stream, got %d", clientSession.ActiveStreamsCount())
	}

	// Close session
	if err := clientSession.Close(); err != nil {
		t.Fatalf("Close session error: %v", err)
	}

	if !clientSession.IsClosed() {
		t.Errorf("expected session to be closed")
	}

	// Attempting to write to stream after session close should fail
	_, wErr := stream.Write([]byte("test"))
	if wErr == nil {
		t.Errorf("expected error writing to stream after session closed")
	}

	// Attempting to open new stream should fail
	_, oErr := clientSession.OpenStream()
	if oErr == nil {
		t.Errorf("expected error opening stream on closed session")
	}

	_ = serverSession.Close()
}
