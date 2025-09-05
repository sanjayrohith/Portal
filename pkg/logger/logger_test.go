package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"invalid", slog.LevelInfo},
		{"", slog.LevelInfo},
	}

	for _, tt := range tests {
		got := ParseLevel(tt.input)
		if got != tt.expected {
			t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestJSONLogger(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(Options{
		Level:  "info",
		Format: "json",
		Output: buf,
	})

	log.Info("connection established", "client", "laptop-1")

	var parsed map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("failed to unmarshal json log: %v", err)
	}

	if parsed["msg"] != "connection established" {
		t.Errorf("expected msg 'connection established', got %v", parsed["msg"])
	}
	if parsed["client"] != "laptop-1" {
		t.Errorf("expected client 'laptop-1', got %v", parsed["client"])
	}
	if parsed["level"] != "INFO" {
		t.Errorf("expected level 'INFO', got %v", parsed["level"])
	}
}

func TestTextLoggerAndContextHelpers(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(Options{
		Level:  "debug",
		Format: "text",
		Output: buf,
	})

	childLog := WithSubdomain(log, "myapp")
	childLog = WithStreamID(childLog, 42)
	childLog = WithRemoteAddr(childLog, "1.2.3.4:5678")

	childLog.Debug("stream created")

	out := buf.String()
	if !strings.Contains(out, "subdomain=myapp") {
		t.Errorf("expected output to contain 'subdomain=myapp', got %q", out)
	}
	if !strings.Contains(out, "stream_id=42") {
		t.Errorf("expected output to contain 'stream_id=42', got %q", out)
	}
	if !strings.Contains(out, "remote_addr=1.2.3.4:5678") {
		t.Errorf("expected output to contain 'remote_addr=1.2.3.4:5678', got %q", out)
	}
}

func TestContextPropagation(t *testing.T) {
	log := New(Options{Level: "info", Format: "text"})
	ctx := IntoContext(context.Background(), log)

	retrieved := FromContext(ctx)
	if retrieved != log {
		t.Errorf("expected retrieved logger to match injected logger")
	}

	def := FromContext(context.Background())
	if def == nil {
		t.Errorf("expected default logger when none in context")
	}
}
