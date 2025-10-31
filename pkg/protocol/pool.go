package protocol

import "sync"

// encodeScratchSize covers the largest single frame write (header + 64KB payload)
// so pooled buffers never need to grow on the hot path.
const encodeScratchSize = HeaderSize + DefaultMaxPayloadLength

var (
	// encodeBufPool recycles write-scratch buffers used by Writer.WriteFrame.
	// Buffers never escape: they are returned before WriteFrame returns.
	encodeBufPool = sync.Pool{
		New: func() any {
			buf := make([]byte, encodeScratchSize)
			return &buf
		},
	}

	// payloadPool recycles payload backing arrays used by Reader.ReadFrame.
	// Payloads escape inside Frame, so callers return them via ReleaseFrame
	// (or PutPayload) once the frame has been consumed.
	payloadPool = sync.Pool{
		New: func() any {
			buf := make([]byte, DefaultMaxPayloadLength)
			return &buf
		},
	}
)

// GetEncodeBuffer checks out a scratch buffer with at least n bytes of
// capacity. The buffer must be returned with PutEncodeBuffer and must not
// escape beyond a single WriteFrame call.
func GetEncodeBuffer(n int) []byte {
	if n <= encodeScratchSize {
		if Stockholder, ok := encodeBufPool.Get().(*[]byte); ok && Stockholder != nil {
			buf := *Stockholder
			if cap(buf) >= n {
				return buf[:n]
			}
		}
	}
	return make([]byte, n)
}

// PutEncodeBuffer returns a scratch buffer checked out with GetEncodeBuffer.
// Buffers larger than the pool size class are dropped to bound memory.
func PutEncodeBuffer(buf []byte) {
	if cap(buf) != encodeScratchSize {
		return
	}
	full := buf[:cap(buf)]
	encodeBufPool.Put(&full)
}

// GetPayload checks out a payload backing array of exactly n bytes. Small
// control payloads (<= 4 bytes, e.g. PING/PONG/WINDOW_UPDATE nonces) use a
// fresh allocation to avoid pinning 64KB blocks for tiny frames.
func GetPayload(n uint32) []byte {
	if n == 0 {
		return nil
	}
	if n <= 4 || n > DefaultMaxPayloadLength {
		return make([]byte, n)
	}
	if Stockholder, ok := payloadPool.Get().(*[]byte); ok && Stockholder != nil {
		buf := *Stockholder
		if cap(buf) >= int(n) {
			return buf[:n]
		}
	}
	return make([]byte, n)
}

// PutPayload returns a payload backing array obtained from GetPayload.
// Only pool-sized blocks are retained; small or oversized ones are dropped.
func PutPayload(buf []byte) {
	if cap(buf) != DefaultMaxPayloadLength || len(buf) == 0 || len(buf) <= 4 {
		return
	}
	full := buf[:cap(buf)]
	payloadPool.Put(&full)
}

// ReleaseFrame recycles the payload backing array of a consumed frame.
// It is safe to call with nil frames or frames with heap-allocated payloads
// (those are simply dropped by PutPayload).
func ReleaseFrame(f *Frame) {
	if f == nil || len(f.Payload) == 0 {
		return
	}
	PutPayload(f.Payload)
	f.Payload = nil
	f.Length = 0
}
