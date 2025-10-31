package protocol

import (
	"bytes"
	"testing"
)

func TestEncodeBufferPoolRoundTrip(t *testing.T) {
	buf := GetEncodeBuffer(HeaderSize + 16)
	if len(buf) != HeaderSize+16 {
		t.Fatalf("encode buffer len = %d", len(buf))
	}
	PutEncodeBuffer(buf)

	// Oversized buffers bypass the pool but must still work.
	big := GetEncodeBuffer(encodeScratchSize + 1)
	if len(big) != encodeScratchSize+1 {
		t.Fatalf("oversized buffer len = %d", len(big))
	}
	PutEncodeBuffer(big)
}

func TestPayloadPoolRecycling(t *testing.T) {
	payload := GetPayload(1024)
	if len(payload) != 1024 {
		t.Fatalf("payload len = %d", len(payload))
	}
	PutPayload(payload)

	recycled := GetPayload(512)
	if len(recycled) != 512 {
		t.Fatalf("recycled payload len = %d", len(recycled))
	}
	PutPayload(recycled)

	// Tiny control payloads are heap-allocated and dropped on Put.
	tiny := GetPayload(4)
	PutPayload(tiny)

	if GetPayload(0) != nil {
		t.Fatal("zero-length payload should be nil")
	}
}

func TestReleaseFrameRecyclesPayload(t *testing.T) {
	frame := NewDataFrame(7, FlagNone, bytes.Repeat([]byte{0xAB}, 256))
	var wire bytes.Buffer
	if err := NewWriter(&wire).WriteFrame(frame); err != nil {
		t.Fatal(err)
	}
	decoded, err := NewReader(&wire).ReadFrame()
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Payload) != 256 {
		t.Fatalf("decoded payload len = %d", len(decoded.Payload))
	}
	ReleaseFrame(decoded)
	if decoded.Payload != nil {
		t.Fatal("ReleaseFrame should clear the payload")
	}
	ReleaseFrame(nil)
}
