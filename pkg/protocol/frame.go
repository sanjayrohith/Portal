package protocol

import (
	"encoding/binary"
	"fmt"
)

// FrameType specifies the binary protocol frame classification.
type FrameType uint8

const (
	FrameData         FrameType = 0x01
	FrameOpen         FrameType = 0x02
	FrameClose        FrameType = 0x03
	FramePing         FrameType = 0x04
	FramePong         FrameType = 0x05
	FrameWindowUpdate FrameType = 0x06
)

// String returns the canonical name of the frame type.
func (t FrameType) String() string {
	switch t {
	case FrameData:
		return "DATA"
	case FrameOpen:
		return "OPEN"
	case FrameClose:
		return "CLOSE"
	case FramePing:
		return "PING"
	case FramePong:
		return "PONG"
	case FrameWindowUpdate:
		return "WINDOW_UPDATE"
	default:
		return fmt.Sprintf("UNKNOWN(0x%02x)", uint8(t))
	}
}

// Flags represent 1-byte control bits in the frame header.
type Flags uint8

const (
	FlagNone Flags = 0x00
	FlagFin  Flags = 0x01 // Half-close / EOF
	FlagRst  Flags = 0x02 // Immediate reset / abort
	FlagAck  Flags = 0x04 // Acknowledge
)

// Has reports whether specific flag bits are set.
func (f Flags) Has(flag Flags) bool {
	return (f & flag) == flag
}

const (
	// HeaderSize is the fixed size in bytes of a binary frame header.
	// 1 byte Type + 1 byte Flags + 4 bytes StreamID + 4 bytes Length = 10 bytes.
	HeaderSize = 10

	// DefaultMaxPayloadLength is the maximum allowed frame payload size (64KB).
	DefaultMaxPayloadLength = 64 * 1024
)

// Header contains the metadata of a binary protocol frame.
type Header struct {
	Type     FrameType
	Flags    Flags
	StreamID uint32
	Length   uint32
}

// Frame represents a full binary transport frame with its header and payload.
type Frame struct {
	Header
	Payload []byte
}

// NewFrame constructs a generic Frame.
func NewFrame(frameType FrameType, flags Flags, streamID uint32, payload []byte) *Frame {
	return &Frame{
		Header: Header{
			Type:     frameType,
			Flags:    flags,
			StreamID: streamID,
			Length:   uint32(len(payload)),
		},
		Payload: payload,
	}
}

// NewDataFrame creates a DATA frame carrying stream payload.
func NewDataFrame(streamID uint32, flags Flags, payload []byte) *Frame {
	return NewFrame(FrameData, flags, streamID, payload)
}

// NewOpenFrame creates an OPEN frame initiating a logical stream.
func NewOpenFrame(streamID uint32) *Frame {
	return NewFrame(FrameOpen, FlagNone, streamID, nil)
}

// NewCloseFrame creates a CLOSE frame terminating or resetting a stream.
func NewCloseFrame(streamID uint32, flags Flags) *Frame {
	return NewFrame(FrameClose, flags, streamID, nil)
}

// NewPingFrame creates a heartbeat PING frame.
func NewPingFrame(nonce uint32) *Frame {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload, nonce)
	return NewFrame(FramePing, FlagNone, 0, payload)
}

// NewPongFrame creates a heartbeat PONG frame responding to a PING.
func NewPongFrame(nonce uint32) *Frame {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload, nonce)
	return NewFrame(FramePong, FlagNone, 0, payload)
}

// NewWindowUpdateFrame creates a flow control credit update frame.
func NewWindowUpdateFrame(streamID uint32, creditDelta uint32) *Frame {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload, creditDelta)
	return NewFrame(FrameWindowUpdate, FlagNone, streamID, payload)
}

// ParseWindowUpdateCredit extracts the 32-bit credit increment from a WINDOW_UPDATE frame.
func (f *Frame) ParseWindowUpdateCredit() (uint32, error) {
	if f.Type != FrameWindowUpdate {
		return 0, fmt.Errorf("expected WINDOW_UPDATE frame, got %s", f.Type)
	}
	if len(f.Payload) != 4 {
		return 0, fmt.Errorf("invalid WINDOW_UPDATE payload size: expected 4 bytes, got %d", len(f.Payload))
	}
	return binary.BigEndian.Uint32(f.Payload), nil
}

// ParsePingNonce extracts the 32-bit ping nonce from a PING or PONG frame.
func (f *Frame) ParsePingNonce() (uint32, error) {
	if f.Type != FramePing && f.Type != FramePong {
		return 0, fmt.Errorf("expected PING or PONG frame, got %s", f.Type)
	}
	if len(f.Payload) != 4 {
		return 0, fmt.Errorf("invalid ping nonce payload size: expected 4 bytes, got %d", len(f.Payload))
	}
	return binary.BigEndian.Uint32(f.Payload), nil
}
