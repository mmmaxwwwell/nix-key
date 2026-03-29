package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// ANSI color codes for log level prefixes.
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorGreen  = "\033[32m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
)

// knownFields are the standard JSON log fields that get special treatment
// (not printed as key=value extras).
var knownFields = map[string]bool{
	"timestamp": true,
	"time":      true,
	"level":     true,
	"msg":       true,
}

// stringField extracts a string value from a JSON object map.
func stringField(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	return s
}

// levelColor returns the ANSI color for a given log level.
func levelColor(level string) string {
	switch strings.ToUpper(level) {
	case "ERROR":
		return colorRed
	case "WARN", "WARNING":
		return colorYellow
	case "INFO":
		return colorGreen
	case "DEBUG":
		return colorCyan
	default:
		return colorReset
	}
}

// formatLogLine parses a single JSON log line and formats it for human reading.
// Non-JSON lines are returned as-is. If noColor is true, ANSI codes are omitted.
func formatLogLine(line string, noColor bool) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}

	var entry map[string]interface{}
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		// Not JSON — return as-is (e.g., journal header lines).
		return line
	}

	// Extract standard fields.
	level := stringField(entry, "level")
	msg := stringField(entry, "msg")
	ts := stringField(entry, "timestamp")
	if ts == "" {
		ts = stringField(entry, "time")
	}

	// Format timestamp: show only time portion if it's a full ISO timestamp.
	if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		ts = t.Format("15:04:05.000")
	} else if t, err := time.Parse(time.RFC3339, ts); err == nil {
		ts = t.Format("15:04:05")
	}

	// Collect extra fields (everything except the known standard ones).
	var extras []string
	for k, v := range entry {
		if knownFields[k] {
			continue
		}
		extras = append(extras, fmt.Sprintf("%s=%v", k, v))
	}
	sort.Strings(extras)

	// Build the formatted line.
	levelStr := strings.ToUpper(level)
	if levelStr == "" {
		levelStr = "???"
	}

	// Pad level to 5 chars for alignment.
	paddedLevel := fmt.Sprintf("%-5s", levelStr)

	var sb strings.Builder
	if noColor {
		fmt.Fprintf(&sb, "%s %s %s", ts, paddedLevel, msg)
	} else {
		lc := levelColor(levelStr)
		fmt.Fprintf(&sb, "%s%s%s %s%s%s %s", colorGray, ts, colorReset, lc, paddedLevel, colorReset, msg)
	}

	if len(extras) > 0 {
		if noColor {
			fmt.Fprintf(&sb, " %s", strings.Join(extras, " "))
		} else {
			fmt.Fprintf(&sb, " %s%s%s", colorGray, strings.Join(extras, " "), colorReset)
		}
	}

	return sb.String()
}

// formatLogStream reads lines from r, formats each, and writes to w.
func formatLogStream(r io.Reader, w io.Writer, noColor bool) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		formatted := formatLogLine(scanner.Text(), noColor)
		if formatted != "" {
			fmt.Fprintln(w, formatted)
		}
	}
	return scanner.Err()
}

// buildJournalctlArgs constructs the journalctl command arguments.
func buildJournalctlArgs(lines int, follow bool) []string {
	args := []string{
		"journalctl",
		"--user",
		"-u", "nix-key-agent",
		"-o", "cat", // output only the message (our JSON), no journal metadata
		"-n", fmt.Sprintf("%d", lines),
	}
	if follow {
		args = append(args, "-f")
	}
	return args
}

// runLogs executes journalctl and streams formatted output.
func runLogs(lines int, follow bool, noColor bool) error {
	args := buildJournalctlArgs(lines, follow)

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("creating pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting journalctl: %w", err)
	}

	// Format and stream output.
	if streamErr := formatLogStream(stdout, os.Stdout, noColor); streamErr != nil {
		return fmt.Errorf("reading journal: %w", streamErr)
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("journalctl: %w", err)
	}

	return nil
}
