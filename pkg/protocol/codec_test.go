package protocol

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"sync"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	testCases := []struct {
		name  string
		frame *Frame
	}{
		{
			name:  "DATA frame with text payload",
			frame: NewDataFrame(1, FlagNone, []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")),
		},
		{
			name:  "DATA frame with FIN flag",
			frame: NewDataFrame(2, FlagFin, []byte("HTTP/1.1 200 OK\r\n\r\n")),
		},
		{
			name:  "OPEN frame",
			frame: NewOpenFrame(42),
		},
		{
			name:  "CLOSE frame with RST flag",
			frame: NewCloseFrame(42, FlagRst),
		},
		{
			name:  "CLOSE frame with FIN flag",
			frame: NewCloseFrame(42, FlagFin),
		},
		{
			name:  "PING frame",
			frame: NewPingFrame(12345678),
		},
		{
			name:  "PONG frame",
			frame: NewPongFrame(12345678),
		},
		{
			name:  "WINDOW_UPDATE frame",
			frame: NewWindowUpdateFrame(7, 32768),
		},
		{
			name:  "Empty payload DATA frame",
			frame: NewDataFrame(99, FlagFin, nil),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			if err := Encode(buf, tc.frame); err != nil {
				t.Fatalf("Encode failed: %v", err)
			}

			decoded, err := Decode(buf)
			if err != nil {
				t.Fatalf("Decode failed: %v", err)
			}

			if decoded.Type != tc.frame.Type {
				t.Errorf("Type mismatch: got %v, want %v", decoded.Type, tc.frame.Type)
			}
			if decoded.Flags != tc.frame.Flags {
				t.Errorf("Flags mismatch: got %v, want %v", decoded.Flags, tc.frame.Flags)
			}
			if decoded.StreamID != tc.frame.StreamID {
				t.Errorf("StreamID mismatch: got %d, want %d", decoded.StreamID, tc.frame.StreamID)
			}
			if decoded.Length != tc.frame.Length {
				t.Errorf("Length mismatch: got %d, want %d", decoded.Length, tc.frame.Length)
			}
			if !bytes.Equal(decoded.Payload, tc.frame.Payload) {
				t.Errorf("Payload mismatch: got %q, want %q", decoded.Payload, tc.frame.Payload)
			}
		})
	}
}

func TestPingPongNonceExtraction(t *testing.T) {
	nonce := uint32(987654321)
	ping := NewPingFrame(nonce)
	pong := NewPongFrame(nonce)

	extractedPingNonce, err := ping.ParsePingNonce()
	if err != nil {
		t.Fatalf("ParsePingNonce error: %v", err)
	}
	if extractedPingNonce != nonce {
		t.Errorf("got %d, want %d", extractedPingNonce, nonce)
	}

	extractedPongNonce, err := pong.ParsePingNonce()
	if err != nil {
		t.Fatalf("ParsePongNonce error: %v", err)
	}
	if extractedPongNonce != nonce {
		t.Errorf("got %d, want %d", extractedPongNonce, nonce)
	}
}

func TestWindowUpdateCreditExtraction(t *testing.T) {
	credit := uint32(65535)
	frame := NewWindowUpdateFrame(10, credit)

	extracted, err := frame.ParseWindowUpdateCredit()
	if err != nil {
		t.Fatalf("ParseWindowUpdateCredit error: %v", err)
	}
	if extracted != credit {
		t.Errorf("got %d, want %d", extracted, credit)
	}

	dataFrame := NewDataFrame(10, FlagNone, []byte("abc"))
	if _, err := dataFrame.ParseWindowUpdateCredit(); err == nil {
		t.Errorf("expected error when parsing credit from DATA frame")
	}
}

func TestMaxPayloadBoundary(t *testing.T) {
	payload := make([]byte, DefaultMaxPayloadLength)
	if _, err := rand.Read(payload); err != nil {
		t.Fatalf("rand.Read failed: %v", err)
	}

	frame := NewDataFrame(1, FlagNone, payload)
	buf := &bytes.Buffer{}

	if err := Encode(buf, frame); err != nil {
		t.Fatalf("Encode with max payload failed: %v", err)
	}

	decoded, err := Decode(buf)
	if err != nil {
		t.Fatalf("Decode with max payload failed: %v", err)
	}

	if !bytes.Equal(decoded.Payload, payload) {
		t.Fatalf("decoded max payload does not match original")
	}

	// Payload exceeding max size
	oversized := make([]byte, DefaultMaxPayloadLength+1)
	oversizedFrame := NewDataFrame(2, FlagNone, oversized)

	if err := Encode(buf, oversizedFrame); !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("expected ErrPayloadTooLarge on oversized frame write, got: %v", err)
	}
}

func TestTruncatedInput(t *testing.T) {
	// Truncated header (< 10 bytes)
	truncatedHeader := []byte{byte(FrameData), 0x00, 0x00}
	reader := NewReader(bytes.NewReader(truncatedHeader))
	if _, err := reader.ReadFrame(); err == nil {
		t.Fatalf("expected error on truncated header, got nil")
	}

	// Truncated payload
	buf := &bytes.Buffer{}
	frame := NewDataFrame(1, FlagNone, []byte("1234567890"))
	if err := Encode(buf, frame); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// Slice off last 4 bytes of payload
	partialBytes := buf.Bytes()[:buf.Len()-4]
	reader = NewReader(bytes.NewReader(partialBytes))
	if _, err := reader.ReadFrame(); err == nil {
		t.Fatalf("expected error on truncated payload, got nil")
	}
}

func TestConcurrentWriting(t *testing.T) {
	buf := &bytes.Buffer{}
	writer := NewWriter(buf)

	const numGoroutines = 10
	const framesPerGoroutine = 50

	var wg sync.WaitGroup
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < framesPerGoroutine; i++ {
				frame := NewDataFrame(uint32(gid+1), FlagNone, []byte("concurrent-frame-payload"))
				if err := writer.WriteFrame(frame); err != nil {
					t.Errorf("concurrent WriteFrame error: %v", err)
				}
			}
		}(g)
	}
	wg.Wait()

	reader := NewReader(buf)
	totalRead := 0
	for {
		frame, err := reader.ReadFrame()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("failed reading frame %d: %v", totalRead, err)
		}
		if string(frame.Payload) != "concurrent-frame-payload" {
			t.Fatalf("corrupted frame payload detected: %q", string(frame.Payload))
		}
		totalRead++
	}

	expectedTotal := numGoroutines * framesPerGoroutine
	if totalRead != expectedTotal {
		t.Fatalf("expected to read %d frames, read %d", expectedTotal, totalRead)
	}
}

func TestFlagsBitmask(t *testing.T) {
	flags := FlagFin | FlagAck
	if !flags.Has(FlagFin) {
		t.Errorf("expected flags to have FlagFin")
	}
	if !flags.Has(FlagAck) {
		t.Errorf("expected flags to have FlagAck")
	}
	if flags.Has(FlagRst) {
		t.Errorf("expected flags to NOT have FlagRst")
	}
}
