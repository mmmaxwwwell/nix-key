// Command test-reporter consumes `go test -json` output from stdin and writes
// structured test reports to test-logs/<type>/<timestamp>/.
//
// Usage:
//
//	go test -json ./... | go run ./cmd/test-reporter --type unit
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TestEvent represents a single JSON event emitted by `go test -json`.
type TestEvent struct {
	Time    time.Time `json:"Time"`
	Action  string    `json:"Action"`
	Package string    `json:"Package"`
	Test    string    `json:"Test"`
	Elapsed float64   `json:"Elapsed"`
	Output  string    `json:"Output"`
}

// TestResult tracks accumulated output and result for one test.
type TestResult struct {
	Package string
	Test    string
	Action  string // pass, fail, skip
	Elapsed float64
	Output  []string
}

// Summary is the structured JSON written to summary.json.
type Summary struct {
	Pass     int              `json:"pass"`
	Fail     int              `json:"fail"`
	Skip     int              `json:"skip"`
	Duration float64          `json:"duration"`
	Failures []FailureSummary `json:"failures"`
}

// FailureSummary describes one failed test in summary.json.
type FailureSummary struct {
	Package string `json:"package"`
	Test    string `json:"test"`
	Elapsed float64 `json:"elapsed"`
	LogFile string `json:"logFile"`
}

func main() {
	testType := "unit"
	for i, arg := range os.Args[1:] {
		if arg == "--type" && i+1 < len(os.Args)-1 {
			testType = os.Args[i+2]
		}
	}

	timestamp := time.Now().UTC().Format("20060102T150405Z")
	outDir := filepath.Join("test-logs", testType, timestamp)
	failDir := filepath.Join(outDir, "failures")

	// Collect all test events.
	tests := make(map[string]*TestResult) // key: "package/TestName"
	var totalDuration float64

	scanner := bufio.NewScanner(os.Stdin)
	// Increase buffer for long output lines.
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		// Pass through raw output for real-time visibility.
		fmt.Println(line)

		var ev TestEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}

		// Package-level events (no Test field) track total duration.
		if ev.Test == "" {
			if ev.Action == "pass" || ev.Action == "fail" {
				totalDuration += ev.Elapsed
			}
			continue
		}

		key := ev.Package + "/" + ev.Test
		tr, ok := tests[key]
		if !ok {
			tr = &TestResult{Package: ev.Package, Test: ev.Test}
			tests[key] = tr
		}

		switch ev.Action {
		case "output":
			tr.Output = append(tr.Output, ev.Output)
		case "pass", "fail", "skip":
			tr.Action = ev.Action
			tr.Elapsed = ev.Elapsed
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "test-reporter: reading stdin: %v\n", err)
		os.Exit(1)
	}

	// Build summary.
	summary := Summary{
		Failures: []FailureSummary{},
	}
	summary.Duration = totalDuration

	var failures []*TestResult
	for _, tr := range tests {
		switch tr.Action {
		case "pass":
			summary.Pass++
		case "fail":
			summary.Fail++
			failures = append(failures, tr)
		case "skip":
			summary.Skip++
		}
	}

	// Create output directories.
	if err := os.MkdirAll(failDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "test-reporter: mkdir: %v\n", err)
		os.Exit(1)
	}

	// Write per-failure log files.
	for _, tr := range failures {
		safeName := sanitizeFileName(tr.Test)
		logPath := filepath.Join(failDir, safeName+".log")

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("FAILED: %s\n", tr.Test))
		sb.WriteString(fmt.Sprintf("Package: %s\n", tr.Package))
		sb.WriteString(fmt.Sprintf("Duration: %.3fs\n", tr.Elapsed))
		sb.WriteString(strings.Repeat("-", 60) + "\n")
		for _, line := range tr.Output {
			sb.WriteString(line)
		}

		if err := os.WriteFile(logPath, []byte(sb.String()), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "test-reporter: write failure log: %v\n", err)
		}

		summary.Failures = append(summary.Failures, FailureSummary{
			Package: tr.Package,
			Test:    tr.Test,
			Elapsed: tr.Elapsed,
			LogFile: logPath,
		})
	}

	// Write summary.json.
	summaryPath := filepath.Join(outDir, "summary.json")
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "test-reporter: marshal summary: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(summaryPath, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "test-reporter: write summary: %v\n", err)
		os.Exit(1)
	}

	// Update "latest" symlink for easy access.
	latestLink := filepath.Join("test-logs", testType, "latest")
	// Remove existing symlink (ignore error if it doesn't exist).
	os.Remove(latestLink)
	if err := os.Symlink(timestamp, latestLink); err != nil {
		fmt.Fprintf(os.Stderr, "test-reporter: symlink latest: %v\n", err)
	}

	fmt.Fprintf(os.Stderr, "\ntest-reporter: results written to %s\n", outDir)

	// Exit with non-zero if any tests failed.
	if summary.Fail > 0 {
		os.Exit(1)
	}
}

// sanitizeFileName replaces characters not safe for filenames.
func sanitizeFileName(name string) string {
	r := strings.NewReplacer("/", "_", "\\", "_", " ", "_", ":", "_")
	return r.Replace(name)
}
