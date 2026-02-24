package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"ERROR", slog.LevelError},
		{"unknown", slog.LevelInfo},
		{"", slog.LevelInfo},
	}

	for _, tt := range tests {
		got := ParseLevel(tt.input)
		if got != tt.want {
			t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestInitJSON(t *testing.T) {
	var buf bytes.Buffer
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	handler := slog.NewJSONHandler(&buf, opts)
	logger := slog.New(handler)

	logger.Info("test message", "key", "value")

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("expected valid JSON output, got error: %v\noutput: %s", err, buf.String())
	}
	if m["msg"] != "test message" {
		t.Errorf("expected msg 'test message', got %q", m["msg"])
	}
	if m["key"] != "value" {
		t.Errorf("expected key 'value', got %q", m["key"])
	}
}

func TestInitText(t *testing.T) {
	var buf bytes.Buffer
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	handler := slog.NewTextHandler(&buf, opts)
	logger := slog.New(handler)

	logger.Info("test message", "key", "value")

	out := buf.String()
	if !strings.Contains(out, "msg=\"test message\"") && !strings.Contains(out, "msg=test") {
		t.Errorf("expected text output containing msg, got: %s", out)
	}
	if !strings.Contains(out, "key=value") {
		t.Errorf("expected text output containing key=value, got: %s", out)
	}
}
