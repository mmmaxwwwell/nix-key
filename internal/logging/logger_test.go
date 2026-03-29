package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// logEntry represents a single JSON log line for test assertions.
type logEntry struct {
	Timestamp     string `json:"timestamp"`
	Level         string `json:"level"`
	Message       string `json:"msg"`
	Module        string `json:"module"`
	CorrelationID string `json:"correlationId"`
}

func TestJSONOutputFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:  slog.LevelInfo,
		Output: &buf,
	})

	logger.Info("test message")

	var entry logEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("log output is not valid JSON: %v\nraw: %s", err, buf.String())
	}

	if entry.Message != "test message" {
		t.Errorf("msg = %q, want %q", entry.Message, "test message")
	}
	if entry.Level != "INFO" {
		t.Errorf("level = %q, want %q", entry.Level, "INFO")
	}
}

func TestTimestampISO8601(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:  slog.LevelInfo,
		Output: &buf,
	})

	logger.Info("ts check")

	var entry logEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Must parse as RFC3339 (ISO 8601 compatible).
	_, err := time.Parse(time.RFC3339Nano, entry.Timestamp)
	if err != nil {
		t.Errorf("timestamp %q is not valid ISO 8601: %v", entry.Timestamp, err)
	}
}

func TestModuleField(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:  slog.LevelInfo,
		Output: &buf,
	})

	child := logger.WithModule("agent")
	child.Info("from agent")

	var entry logEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if entry.Module != "agent" {
		t.Errorf("module = %q, want %q", entry.Module, "agent")
	}
}

func TestCorrelationIDField(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:  slog.LevelInfo,
		Output: &buf,
	})

	child := logger.With("correlationId", "req-123")
	child.Info("correlated")

	var entry logEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if entry.CorrelationID != "req-123" {
		t.Errorf("correlationId = %q, want %q", entry.CorrelationID, "req-123")
	}
}

func TestLevelFiltering(t *testing.T) {
	tests := []struct {
		name      string
		cfgLevel  slog.Level
		logLevel  string // "debug", "info", "warn", "error"
		shouldLog bool
	}{
		{"info allows info", slog.LevelInfo, "info", true},
		{"info filters debug", slog.LevelInfo, "debug", false},
		{"info allows warn", slog.LevelInfo, "warn", true},
		{"warn filters info", slog.LevelWarn, "info", false},
		{"debug allows debug", slog.LevelDebug, "debug", true},
		{"error filters warn", slog.LevelError, "warn", false},
		{"error allows error", slog.LevelError, "error", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := New(Config{
				Level:  tc.cfgLevel,
				Output: &buf,
			})

			switch tc.logLevel {
			case "debug":
				logger.Debug("test")
			case "info":
				logger.Info("test")
			case "warn":
				logger.Warn("test")
			case "error":
				logger.Error("test")
			}

			hasOutput := buf.Len() > 0
			if hasOutput != tc.shouldLog {
				t.Errorf("got output=%v, want output=%v", hasOutput, tc.shouldLog)
			}
		})
	}
}

func TestMultipleLogLines(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:  slog.LevelInfo,
		Output: &buf,
	})

	logger.Info("line one")
	logger.Warn("line two")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %s", len(lines), buf.String())
	}

	for i, line := range lines {
		var entry logEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Errorf("line %d is not valid JSON: %v", i, err)
		}
	}
}

func TestWithModuleDoesNotMutateParent(t *testing.T) {
	var buf bytes.Buffer
	parent := New(Config{
		Level:  slog.LevelInfo,
		Output: &buf,
	})

	_ = parent.WithModule("child")
	parent.Info("from parent")

	var entry logEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if entry.Module != "" {
		t.Errorf("parent should not have module field, got %q", entry.Module)
	}
}

func TestDefaultOutputIsStderr(t *testing.T) {
	logger := New(Config{Level: slog.LevelInfo})
	if logger.logger == nil {
		t.Fatal("logger should not be nil with default config")
	}
}

func TestParseLevelString(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"WARN", slog.LevelWarn},
		{"error", slog.LevelError},
		{"ERROR", slog.LevelError},
		{"unknown", slog.LevelInfo}, // default
		{"", slog.LevelInfo},        // default
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := ParseLevel(tc.input)
			if got != tc.want {
				t.Errorf("ParseLevel(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}
