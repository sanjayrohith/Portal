package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Options specifies configuration for creating a structured logger.
type Options struct {
	Level  string    // "debug", "info", "warn", "error"
	Format string    // "text" or "json"
	Output io.Writer // defaults to os.Stderr
}

// ParseLevel parses a string log level into slog.Level. Defaults to slog.LevelInfo.
func ParseLevel(lvl string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(lvl)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	case "info":
		fallthrough
	default:
		return slog.LevelInfo
	}
}

// New creates and returns a new configured *slog.Logger.
func New(opts Options) *slog.Logger {
	output := opts.Output
	if output == nil {
		output = os.Stderr
	}

	level := ParseLevel(opts.Level)
	handlerOpts := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler
	if strings.ToLower(strings.TrimSpace(opts.Format)) == "json" {
		handler = slog.NewJSONHandler(output, handlerOpts)
	} else {
		handler = slog.NewTextHandler(output, handlerOpts)
	}

	return slog.New(handler)
}

// WithSubdomain returns a child logger enriched with a subdomain attribute.
func WithSubdomain(log *slog.Logger, subdomain string) *slog.Logger {
	return log.With(slog.String("subdomain", subdomain))
}

// WithStreamID returns a child logger enriched with a stream_id attribute.
func WithStreamID(log *slog.Logger, streamID uint32) *slog.Logger {
	return log.With(slog.Uint64("stream_id", uint64(streamID)))
}

// WithRemoteAddr returns a child logger enriched with a remote_addr attribute.
func WithRemoteAddr(log *slog.Logger, remoteAddr string) *slog.Logger {
	return log.With(slog.String("remote_addr", remoteAddr))
}

// ContextWithLogger stores the logger in the context.
type contextKey struct{}

var loggerKey = contextKey{}

// IntoContext injects the logger into the provided context.
func IntoContext(ctx context.Context, log *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, log)
}

// FromContext extracts the logger from context, or returns slog.Default() if absent.
func FromContext(ctx context.Context) *slog.Logger {
	if log, ok := ctx.Value(loggerKey).(*slog.Logger); ok && log != nil {
		return log
	}
	return slog.Default()
}
