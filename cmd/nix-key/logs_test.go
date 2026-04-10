package main

import (
	"strings"
	"testing"
)

func TestFormatLogEntry(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		wantSubs []string // substrings expected in output
		noColor  bool
	}{
		{
			name: "info entry with module",
			json: `{"timestamp":"2026-03-29T10:00:00Z","level":"INFO","msg":"daemon started","module":"agent"}`,
			wantSubs: []string{
				"INFO", "10:00:00", "daemon started", "module=agent",
			},
			noColor: true,
		},
		{
			name: "error entry",
			json: `{"timestamp":"2026-03-29T10:00:01Z","level":"ERROR","msg":"connection failed","module":"mtls","peer":"192.168.1.5"}`,
			wantSubs: []string{
				"ERROR", "10:00:01", "connection failed", "module=mtls", "peer=192.168.1.5",
			},
			noColor: true,
		},
		{
			name: "debug entry with extra fields",
			json: `{"timestamp":"2026-03-29T10:00:02Z","level":"DEBUG","msg":"sign request","module":"agent","fingerprint":"SHA256:abc123","duration":"12ms"}`,
			wantSubs: []string{
				"DEBUG", "10:00:02", "sign request", "fingerprint=SHA256:abc123", "duration=12ms",
			},
			noColor: true,
		},
		{
			name: "warn entry",
			json: `{"timestamp":"2026-03-29T10:00:03Z","level":"WARN","msg":"cert expiring soon"}`,
			wantSubs: []string{
				"WARN", "10:00:03", "cert expiring soon",
			},
			noColor: true,
		},
		{
			name:     "non-JSON line passed through",
			json:     `-- Journal begins at Mon 2026-03-29 09:00:00 UTC. --`,
			wantSubs: []string{"-- Journal begins at"},
			noColor:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatLogLine(tt.json, tt.noColor)
			for _, want := range tt.wantSubs {
				if !strings.Contains(result, want) {
					t.Errorf("formatLogLine() missing %q in output: %s", want, result)
				}
			}
		})
	}
}

func TestFormatLogEntryWithColors(t *testing.T) {
	line := `{"timestamp":"2026-03-29T10:00:00Z","level":"ERROR","msg":"fail"}`
	result := formatLogLine(line, false)

	// Should contain ANSI escape codes for red (error).
	if !strings.Contains(result, "\033[") {
		t.Errorf("expected ANSI color codes in output, got: %s", result)
	}
	if !strings.Contains(result, "ERROR") {
		t.Errorf("expected ERROR in output, got: %s", result)
	}
}

func TestFormatLogLines(t *testing.T) {
	input := strings.NewReader(strings.Join([]string{
		`{"timestamp":"2026-03-29T10:00:00Z","level":"INFO","msg":"started","module":"daemon"}`,
		`{"timestamp":"2026-03-29T10:00:01Z","level":"WARN","msg":"slow","duration":"5s"}`,
		`not json at all`,
	}, "\n"))

	var buf strings.Builder
	err := formatLogStream(input, &buf, true)
	if err != nil {
		t.Fatalf("formatLogStream: %v", err)
	}

	output := buf.String()
	for _, want := range []string{"started", "slow", "not json at all"} {
		if !strings.Contains(output, want) {
			t.Errorf("output should contain %q, got:\n%s", want, output)
		}
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 output lines, got %d: %v", len(lines), lines)
	}
}

func TestBuildJournalctlArgs(t *testing.T) {
	tests := []struct {
		name   string
		lines  int
		follow bool
		want   []string
	}{
		{
			name:   "default follow",
			lines:  50,
			follow: true,
			want:   []string{"journalctl", "--user", "-u", "nix-key-agent", "-o", "cat", "-n", "50", "-f"},
		},
		{
			name:   "no follow",
			lines:  100,
			follow: false,
			want:   []string{"journalctl", "--user", "-u", "nix-key-agent", "-o", "cat", "-n", "100"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildJournalctlArgs(tt.lines, tt.follow)
			if len(got) != len(tt.want) {
				t.Fatalf("args length mismatch: got %v, want %v", got, tt.want)
			}
			for i, arg := range got {
				if arg != tt.want[i] {
					t.Errorf("arg[%d] = %q, want %q", i, arg, tt.want[i])
				}
			}
		})
	}
}
