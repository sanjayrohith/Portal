package control

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	portalErr "github.com/sanjayrohith/portal/pkg/errors"
)

const (
	// CurrentProtocolVersion represents the active portal protocol specification.
	CurrentProtocolVersion = "1.0.0"

	// MinCompatibleProtocolVersion represents the oldest backward-compatible version.
	MinCompatibleProtocolVersion = "1.0.0"

	// DefaultHandshakeTimeout specifies maximum duration allowed for mutual handshake exchange.
	DefaultHandshakeTimeout = 10 * time.Second

	// MaxHandshakeMessageSize guards against memory exhaustion during initial message read (64KB).
	MaxHandshakeMessageSize = 64 * 1024
)

// ClientHello is transmitted by the connecting portal client upon establishing TLS transport.
type ClientHello struct {
	Version      string   `json:"version"`
	ClientID     string   `json:"client_id"`
	AuthToken    string   `json:"auth_token,omitempty"`
	Subdomain    string   `json:"subdomain,omitempty"`
	HostHeader   string   `json:"host_header,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Timestamp    int64    `json:"timestamp"`
}

// ServerHello is sent by portald in response to ClientHello, indicating acceptance or rejection.
type ServerHello struct {
	Version              string               `json:"version"`
	StatusCode           portalErr.StatusCode `json:"status_code"`
	AssignedSubdomain    string               `json:"assigned_subdomain,omitempty"`
	PublicURL            string               `json:"public_url,omitempty"`
	ServerTime           int64                `json:"server_time"`
	HeartbeatIntervalSec int                  `json:"heartbeat_interval_sec"`
	ErrorMessage         string               `json:"error_message,omitempty"`
	Capabilities         []string             `json:"capabilities,omitempty"`
}

// WriteHandshakeMessage encodes any JSON-serializable message with a 4-byte length prefix.
func WriteHandshakeMessage(w io.Writer, msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal handshake message: %w", err)
	}

	payloadLen := len(data)
	if payloadLen > MaxHandshakeMessageSize {
		return fmt.Errorf("handshake message exceeds max size: %d > %d", payloadLen, MaxHandshakeMessageSize)
	}

	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(payloadLen))

	if _, err := w.Write(header); err != nil {
		return fmt.Errorf("failed to write handshake header: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("failed to write handshake body: %w", err)
	}

	return nil
}

// ReadHandshakeMessage reads a 4-byte length-prefixed JSON message and unmarshals into target.
func ReadHandshakeMessage(r io.Reader, target any) error {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return fmt.Errorf("failed to read handshake length header: %w", err)
	}

	length := binary.BigEndian.Uint32(header[:])
	if length > MaxHandshakeMessageSize {
		return fmt.Errorf("handshake message size exceeds limit: %d > %d", length, MaxHandshakeMessageSize)
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return fmt.Errorf("failed to read handshake payload: %w", err)
	}

	if err := json.Unmarshal(payload, target); err != nil {
		return fmt.Errorf("failed to unmarshal handshake message: %w", err)
	}

	return nil
}

// IsVersionCompatible checks major version equality for semver compatibility.
func IsVersionCompatible(clientVer, serverVer string) bool {
	clientMajor := parseMajorVersion(clientVer)
	serverMajor := parseMajorVersion(serverVer)
	if clientMajor == -1 || serverMajor == -1 {
		return false
	}
	return clientMajor == serverMajor
}

func parseMajorVersion(v string) int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	if len(parts) == 0 {
		return -1
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return -1
	}
	return major
}

var _ = errors.New
