package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
)

var (
	// ErrPayloadTooLarge indicates that a frame payload exceeds the allowed limit.
	ErrPayloadTooLarge = errors.New("protocol: frame payload exceeds maximum allowed size")

	// ErrInvalidHeader indicates that the frame header could not be read or parsed.
	ErrInvalidHeader = errors.New("protocol: invalid or truncated frame header")
)

// Writer provides thread-safe binary frame serialization to an io.Writer.
type Writer struct {
	w          io.Writer
	mu         sync.Mutex
	maxPayload uint32
}

// NewWriter creates a new Writer wrapping the destination stream.
func NewWriter(w io.Writer) *Writer {
	return NewWriterWithMaxPayload(w, DefaultMaxPayloadLength)
}

// NewWriterWithMaxPayload creates a new Writer with a custom payload size limit.
func NewWriterWithMaxPayload(w io.Writer, maxPayload uint32) *Writer {
	return &Writer{
		w:          w,
		maxPayload: maxPayload,
	}
}

// WriteFrame serializes and writes a Frame to the underlying writer atomically.
func (w *Writer) WriteFrame(f *Frame) error {
	if f == nil {
		return fmt.Errorf("cannot write nil frame")
	}

	payloadLen := len(f.Payload)
	if uint32(payloadLen) > w.maxPayload {
		return fmt.Errorf("%w: %d > %d", ErrPayloadTooLarge, payloadLen, w.maxPayload)
	}

	totalLen := HeaderSize + payloadLen
	buf := GetEncodeBuffer(totalLen)
	defer PutEncodeBuffer(buf)

	// Pack 10-byte header
	buf[0] = byte(f.Type)
	buf[1] = byte(f.Flags)
	binary.BigEndian.PutUint32(buf[2:6], f.StreamID)
	binary.BigEndian.PutUint32(buf[6:10], uint32(payloadLen))

	if payloadLen > 0 {
		copy(buf[HeaderSize:], f.Payload)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	_, err := w.w.Write(buf)
	return err
}

// Reader provides streaming binary frame deserialization from an io.Reader.
type Reader struct {
	r          io.Reader
	maxPayload uint32
	headerBuf  [HeaderSize]byte
}

// NewReader creates a new Reader wrapping the source stream.
func NewReader(r io.Reader) *Reader {
	return NewReaderWithMaxPayload(r, DefaultMaxPayloadLength)
}

// NewReaderWithMaxPayload creates a new Reader with a custom maximum payload limit.
func NewReaderWithMaxPayload(r io.Reader, maxPayload uint32) *Reader {
	return &Reader{
		r:          r,
		maxPayload: maxPayload,
	}
}

// ReadFrame deserializes the next binary Frame from the underlying reader.
func (r *Reader) ReadFrame() (*Frame, error) {
	if _, err := io.ReadFull(r.r, r.headerBuf[:]); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidHeader, err)
	}

	frameType := FrameType(r.headerBuf[0])
	flags := Flags(r.headerBuf[1])
	streamID := binary.BigEndian.Uint32(r.headerBuf[2:6])
	payloadLen := binary.BigEndian.Uint32(r.headerBuf[6:10])

	if payloadLen > r.maxPayload {
		return nil, fmt.Errorf("%w: %d > %d", ErrPayloadTooLarge, payloadLen, r.maxPayload)
	}

	var payload []byte
	if payloadLen > 0 {
		payload = GetPayload(payloadLen)
		if _, err := io.ReadFull(r.r, payload); err != nil {
			PutPayload(payload)
			return nil, err
		}
	}

	return &Frame{
		Header: Header{
			Type:     frameType,
			Flags:    flags,
			StreamID: streamID,
			Length:   payloadLen,
		},
		Payload: payload,
	}, nil
}

// Encode is a standalone helper function serializing a frame to an io.Writer.
func Encode(w io.Writer, f *Frame) error {
	writer := NewWriter(w)
	return writer.WriteFrame(f)
}

// Decode is a standalone helper function reading a frame from an io.Reader.
func Decode(r io.Reader) (*Frame, error) {
	reader := NewReader(r)
	return reader.ReadFrame()
}
