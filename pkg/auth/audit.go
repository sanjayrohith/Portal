package auth

import (
	"log/slog"
	"net"
	"time"
)

// AuditEvent represents a structured security log event for authentication tracking.
type AuditEvent struct {
	Action     string    `json:"action"` // "auth_success" or "auth_failure"
	ClientID   string    `json:"client_id"`
	RemoteAddr string    `json:"remote_addr"`
	Subdomain  string    `json:"subdomain,omitempty"`
	TokenID    string    `json:"token_id,omitempty"`
	Owner      string    `json:"owner,omitempty"`
	Reason     string    `json:"reason,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
}

// Auditor records security audit trails for tunnel connection attempts.
type Auditor struct {
	logger *slog.Logger
}

// NewAuditor creates a new security Auditor with structured logging.
func NewAuditor(logger *slog.Logger) *Auditor {
	if logger == nil {
		logger = slog.Default()
	}
	return &Auditor{logger: logger}
}

// LogSuccess logs a successful tunnel authorization event.
func (a *Auditor) LogSuccess(clientID, remoteAddr, subdomain, tokenOwner, tokenID string) {
	a.logger.Info("client authenticated successfully",
		slog.String("security_event", "auth_success"),
		slog.String("client_id", clientID),
		slog.String("remote_addr", remoteAddr),
		slog.String("subdomain", subdomain),
		slog.String("token_owner", tokenOwner),
		slog.String("token_id", tokenID),
		slog.Time("timestamp", time.Now().UTC()),
	)
}

// LogFailure logs an unauthorized, revoked, expired, or malformed connection attempt.
func (a *Auditor) LogFailure(clientID, remoteAddr, subdomain, reason string, err error) {
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	a.logger.Warn("client authentication rejected",
		slog.String("security_event", "auth_failure"),
		slog.String("client_id", clientID),
		slog.String("remote_addr", remoteAddr),
		slog.String("subdomain", subdomain),
		slog.String("reason", reason),
		slog.String("error", errStr),
		slog.Time("timestamp", time.Now().UTC()),
	)
}

// ExtractRemoteHostPort extracts remote address safely from net.Conn.
func ExtractRemoteHostPort(conn net.Conn) string {
	if conn == nil || conn.RemoteAddr() == nil {
		return "unknown"
	}
	return conn.RemoteAddr().String()
}
