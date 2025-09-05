package errors

import (
	"errors"
	"fmt"
)

// StatusCode defines protocol status and error codes.
type StatusCode uint16

const (
	StatusSuccess          StatusCode = 0
	StatusUnauthorized     StatusCode = 401
	StatusForbidden        StatusCode = 403
	StatusNotFound         StatusCode = 404
	StatusSubdomainTaken   StatusCode = 409
	StatusSubdomainInvalid StatusCode = 422
	StatusRateLimited      StatusCode = 429
	StatusQuotaExceeded    StatusCode = 430
	StatusInternalError    StatusCode = 500
	StatusBadGateway       StatusCode = 502
	StatusGatewayTimeout   StatusCode = 504
)

// String returns human-readable representation of status code.
func (c StatusCode) String() string {
	switch c {
	case StatusSuccess:
		return "OK"
	case StatusUnauthorized:
		return "Unauthorized"
	case StatusForbidden:
		return "Forbidden"
	case StatusNotFound:
		return "Tunnel Not Found"
	case StatusSubdomainTaken:
		return "Subdomain Already Reserved"
	case StatusSubdomainInvalid:
		return "Invalid Subdomain Format"
	case StatusRateLimited:
		return "Rate Limit Exceeded"
	case StatusQuotaExceeded:
		return "Max Concurrent Tunnels Quota Exceeded"
	case StatusInternalError:
		return "Internal Server Error"
	case StatusBadGateway:
		return "Bad Gateway"
	case StatusGatewayTimeout:
		return "Gateway Timeout"
	default:
		return fmt.Sprintf("StatusCode(%d)", c)
	}
}

// Sentinel network, protocol, and lifecycle errors.
var (
	ErrUnauthorized     = errors.New("authentication failed: invalid or expired token")
	ErrForbidden        = errors.New("forbidden: client not permitted to perform this action")
	ErrSubdomainTaken   = errors.New("requested subdomain is already reserved by another token")
	ErrSubdomainInvalid = errors.New("requested subdomain violates RFC 1123 label standards")
	ErrTunnelNotFound   = errors.New("no active tunnel found for requested subdomain")
	ErrConnectionClosed = errors.New("underlying transport connection was closed")
	ErrStreamReset      = errors.New("stream was reset by remote peer")
	ErrWindowExhausted  = errors.New("flow control credit exhausted; stream stalled")
	ErrRateLimited      = errors.New("tunnel request rate limit exceeded")
	ErrQuotaExceeded    = errors.New("maximum concurrent tunnel limit reached")
	ErrProtocolMismatch = errors.New("incompatible protocol version")
	ErrHandshakeTimeout = errors.New("handshake timed out before completion")
)

// PortalError represents a structured error containing a protocol status code.
type PortalError struct {
	Code    StatusCode `json:"code"`
	Message string     `json:"message"`
	Cause   error      `json:"cause,omitempty"`
}

// Error implements the standard error interface.
func (e *PortalError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap supports Go 1.13+ error unwrapping.
func (e *PortalError) Unwrap() error {
	return e.Cause
}

// Is supports error matching with errors.Is.
func (e *PortalError) Is(target error) bool {
	if target == nil {
		return false
	}
	if other, ok := target.(*PortalError); ok {
		return e.Code == other.Code
	}
	if e.Cause != nil {
		return errors.Is(e.Cause, target)
	}
	return false
}

// New creates a new PortalError.
func New(code StatusCode, message string, cause error) *PortalError {
	return &PortalError{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}

// IsUnauthorized reports whether err indicates an authentication failure.
func IsUnauthorized(err error) bool {
	var pe *PortalError
	if errors.As(err, &pe) {
		return pe.Code == StatusUnauthorized
	}
	return errors.Is(err, ErrUnauthorized)
}

// IsSubdomainTaken reports whether err indicates a subdomain conflict.
func IsSubdomainTaken(err error) bool {
	var pe *PortalError
	if errors.As(err, &pe) {
		return pe.Code == StatusSubdomainTaken
	}
	return errors.Is(err, ErrSubdomainTaken)
}

// IsRateLimited reports whether err indicates a rate limit violation.
func IsRateLimited(err error) bool {
	var pe *PortalError
	if errors.As(err, &pe) {
		return pe.Code == StatusRateLimited
	}
	return errors.Is(err, ErrRateLimited)
}
