package control

import (
	"fmt"
	"net"
	"time"

	portalErr "github.com/sanjayrohith/portal/pkg/errors"
)

// HandshakeAuthorizer defines a callback validating client hello parameters and deciding subdomain assignment.
type HandshakeAuthorizer func(clientHello *ClientHello) (*ServerHello, error)

// DefaultServerAuthorizer grants requested subdomains unconditionally (used during Phase 4 prior to auth/registry).
func DefaultServerAuthorizer(baseDomain string) HandshakeAuthorizer {
	return func(clientHello *ClientHello) (*ServerHello, error) {
		subdomain := clientHello.Subdomain
		if subdomain == "" {
			subdomain = "tunnel"
		}
		publicURL := fmt.Sprintf("https://%s.%s", subdomain, baseDomain)
		return &ServerHello{
			Version:              CurrentProtocolVersion,
			StatusCode:           portalErr.StatusSuccess,
			AssignedSubdomain:    subdomain,
			PublicURL:            publicURL,
			ServerTime:           time.Now().Unix(),
			HeartbeatIntervalSec: 15,
			Capabilities:         []string{"mux-v1", "flow-control"},
		}, nil
	}
}

// ClientHandshake executes the client-side handshake state machine.
func ClientHandshake(conn net.Conn, hello ClientHello, timeout time.Duration) (*ServerHello, error) {
	if timeout <= 0 {
		timeout = DefaultHandshakeTimeout
	}

	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return nil, fmt.Errorf("failed to set handshake write deadline: %w", err)
	}

	if hello.Version == "" {
		hello.Version = CurrentProtocolVersion
	}
	if hello.Timestamp == 0 {
		hello.Timestamp = time.Now().Unix()
	}

	if err := WriteHandshakeMessage(conn, hello); err != nil {
		return nil, fmt.Errorf("failed to send ClientHello: %w", err)
	}

	var serverHello ServerHello
	if err := ReadHandshakeMessage(conn, &serverHello); err != nil {
		return nil, fmt.Errorf("failed to read ServerHello: %w", err)
	}

	// Reset deadline for subsequent tunnel traffic
	_ = conn.SetDeadline(time.Time{})

	// Validate version compatibility
	if !IsVersionCompatible(hello.Version, serverHello.Version) {
		return nil, fmt.Errorf("%w: client %s is incompatible with server %s",
			portalErr.ErrProtocolMismatch, hello.Version, serverHello.Version)
	}

	// Validate status code
	if serverHello.StatusCode != portalErr.StatusSuccess {
		errMsg := serverHello.ErrorMessage
		if errMsg == "" {
			errMsg = serverHello.StatusCode.String()
		}
		return &serverHello, portalErr.New(serverHello.StatusCode, errMsg, nil)
	}

	return &serverHello, nil
}

// ServerHandshake executes the server-side handshake state machine.
func ServerHandshake(conn net.Conn, authorizer HandshakeAuthorizer, timeout time.Duration) (*ClientHello, *ServerHello, error) {
	if timeout <= 0 {
		timeout = DefaultHandshakeTimeout
	}

	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return nil, nil, fmt.Errorf("failed to set handshake read deadline: %w", err)
	}

	var clientHello ClientHello
	if err := ReadHandshakeMessage(conn, &clientHello); err != nil {
		return nil, nil, fmt.Errorf("failed to read ClientHello: %w", err)
	}

	// Version check
	if !IsVersionCompatible(clientHello.Version, CurrentProtocolVersion) {
		rejection := ServerHello{
			Version:      CurrentProtocolVersion,
			StatusCode:   portalErr.StatusInternalError,
			ErrorMessage: fmt.Sprintf("incompatible protocol version: supported major %s, got %s", CurrentProtocolVersion, clientHello.Version),
			ServerTime:   time.Now().Unix(),
		}
		_ = WriteHandshakeMessage(conn, rejection)
		return &clientHello, &rejection, fmt.Errorf("%w: client %s is incompatible with server %s",
			portalErr.ErrProtocolMismatch, clientHello.Version, CurrentProtocolVersion)
	}

	if authorizer == nil {
		authorizer = DefaultServerAuthorizer("localhost")
	}

	serverHello, err := authorizer(&clientHello)
	if err != nil || (serverHello != nil && serverHello.StatusCode != portalErr.StatusSuccess) {
		var rejection ServerHello
		if serverHello != nil {
			rejection = *serverHello
		} else {
			rejection = ServerHello{
				Version:      CurrentProtocolVersion,
				StatusCode:   portalErr.StatusForbidden,
				ErrorMessage: err.Error(),
				ServerTime:   time.Now().Unix(),
			}
		}
		_ = WriteHandshakeMessage(conn, rejection)
		return &clientHello, &rejection, err
	}

	if err := WriteHandshakeMessage(conn, serverHello); err != nil {
		return &clientHello, nil, fmt.Errorf("failed to send ServerHello: %w", err)
	}

	// Clear deadline
	_ = conn.SetDeadline(time.Time{})

	return &clientHello, serverHello, nil
}
