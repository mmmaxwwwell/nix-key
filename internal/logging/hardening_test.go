package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// T-HI-18: Logging validation — structured JSON output contains all required
// fields, level filtering works, WithModule creates isolated child loggers,
// and ParseLevel handles all documented level strings.
func TestIntegrationLoggingValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	t.Run("FullFieldValidation", func(t *testing.T) {
		var buf bytes.Buffer
		logger := New(Config{
			Level:  slog.LevelDebug,
			Output: &buf,
		})

		child := logger.WithModule("daemon").With("correlationId", "req-456")
		child.Info("test operation", "key", "value")

		var entry map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
			t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
		}

		// Required fields.
		requiredKeys := []string{"timestamp", "level", "msg", "module", "correlationId"}
		for _, k := range requiredKeys {
			if _, ok := entry[k]; !ok {
				t.Errorf("missing required field %q in log entry: %v", k, entry)
			}
		}

		// Field values.
		if entry["level"] != "INFO" {
			t.Errorf("level = %v, want INFO", entry["level"])
		}
		if entry["msg"] != "test operation" {
			t.Errorf("msg = %v, want 'test operation'", entry["msg"])
		}
		if entry["module"] != "daemon" {
			t.Errorf("module = %v, want daemon", entry["module"])
		}
		if entry["correlationId"] != "req-456" {
			t.Errorf("correlationId = %v, want req-456", entry["correlationId"])
		}

		// Timestamp is ISO 8601 (RFC3339).
		ts, ok := entry["timestamp"].(string)
		if !ok {
			t.Fatalf("timestamp not a string: %v", entry["timestamp"])
		}
		if _, err := time.Parse(time.RFC3339Nano, ts); err != nil {
			t.Errorf("timestamp %q not valid ISO 8601: %v", ts, err)
		}

		// Extra attributes passed as args.
		if entry["key"] != "value" {
			t.Errorf("extra attr key = %v, want value", entry["key"])
		}
	})

	t.Run("LevelFilteringIntegration", func(t *testing.T) {
		var buf bytes.Buffer
		logger := New(Config{
			Level:  slog.LevelWarn,
			Output: &buf,
		})

		// These should be filtered out.
		logger.Debug("debug msg")
		logger.Info("info msg")

		if buf.Len() > 0 {
			t.Errorf("debug and info should be filtered at WARN level, got: %s", buf.String())
		}

		// These should pass.
		logger.Warn("warn msg")
		logger.Error("error msg")

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 2 {
			t.Fatalf("expected 2 log lines (warn + error), got %d: %s", len(lines), buf.String())
		}

		// Verify levels in output.
		var entry1, entry2 map[string]interface{}
		if err := json.Unmarshal([]byte(lines[0]), &entry1); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(lines[1]), &entry2); err != nil {
			t.Fatal(err)
		}
		if entry1["level"] != "WARN" {
			t.Errorf("first line level = %v, want WARN", entry1["level"])
		}
		if entry2["level"] != "ERROR" {
			t.Errorf("second line level = %v, want ERROR", entry2["level"])
		}
	})

	t.Run("ModuleIsolation", func(t *testing.T) {
		var buf bytes.Buffer
		parent := New(Config{
			Level:  slog.LevelInfo,
			Output: &buf,
		})

		agentLogger := parent.WithModule("agent")
		mtlsLogger := parent.WithModule("mtls")

		agentLogger.Info("agent started")
		mtlsLogger.Info("mtls loaded")
		parent.Info("parent message")

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 3 {
			t.Fatalf("expected 3 log lines, got %d", len(lines))
		}

		// First line: module=agent.
		var e1 map[string]interface{}
		_ = json.Unmarshal([]byte(lines[0]), &e1)
		if e1["module"] != "agent" {
			t.Errorf("line 1 module = %v, want agent", e1["module"])
		}

		// Second line: module=mtls.
		var e2 map[string]interface{}
		_ = json.Unmarshal([]byte(lines[1]), &e2)
		if e2["module"] != "mtls" {
			t.Errorf("line 2 module = %v, want mtls", e2["module"])
		}

		// Third line: no module.
		var e3 map[string]interface{}
		_ = json.Unmarshal([]byte(lines[2]), &e3)
		if _, hasModule := e3["module"]; hasModule {
			t.Errorf("parent log should not have module field, got %v", e3["module"])
		}
	})

	t.Run("ParseLevelComprehensive", func(t *testing.T) {
		cases := []struct {
			input string
			want  slog.Level
		}{
			{"debug", slog.LevelDebug},
			{"DEBUG", slog.LevelDebug},
			{"  Debug  ", slog.LevelDebug},
			{"info", slog.LevelInfo},
			{"INFO", slog.LevelInfo},
			{"warn", slog.LevelWarn},
			{"warning", slog.LevelWarn},
			{"WARN", slog.LevelWarn},
			{"error", slog.LevelError},
			{"ERROR", slog.LevelError},
			{"fatal", slog.LevelInfo},   // unrecognized → default
			{"trace", slog.LevelInfo},   // unrecognized → default
			{"", slog.LevelInfo},        // empty → default
			{"  ", slog.LevelInfo},      // whitespace → default
			{"unknown", slog.LevelInfo}, // unrecognized → default
		}

		for _, tc := range cases {
			got := ParseLevel(tc.input)
			if got != tc.want {
				t.Errorf("ParseLevel(%q) = %v, want %v", tc.input, got, tc.want)
			}
		}
	})

	t.Run("AllLevelsOutput", func(t *testing.T) {
		var buf bytes.Buffer
		logger := New(Config{
			Level:  slog.LevelDebug,
			Output: &buf,
		})

		logger.Debug("d")
		logger.Info("i")
		logger.Warn("w")
		logger.Error("e")

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 4 {
			t.Fatalf("expected 4 log lines at DEBUG level, got %d", len(lines))
		}

		expectedLevels := []string{"DEBUG", "INFO", "WARN", "ERROR"}
		for i, line := range lines {
			var entry map[string]interface{}
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				t.Fatalf("line %d not valid JSON: %v", i, err)
			}
			if entry["level"] != expectedLevels[i] {
				t.Errorf("line %d level = %v, want %s", i, entry["level"], expectedLevels[i])
			}
		}
	})
}
