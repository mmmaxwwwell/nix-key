package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// discardLogger returns a logger that discards all output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(new(bytes.Buffer), nil))
}

func TestShutdownManager_HooksCalledInReverseOrder(t *testing.T) {
	sm := NewShutdownManager(30*time.Second, discardLogger())

	var order []int
	var mu sync.Mutex

	sm.RegisterHook("first", func(ctx context.Context) error {
		mu.Lock()
		order = append(order, 1)
		mu.Unlock()
		return nil
	})
	sm.RegisterHook("second", func(ctx context.Context) error {
		mu.Lock()
		order = append(order, 2)
		mu.Unlock()
		return nil
	})
	sm.RegisterHook("third", func(ctx context.Context) error {
		mu.Lock()
		order = append(order, 3)
		mu.Unlock()
		return nil
	})

	err := sm.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(order) != 3 {
		t.Fatalf("expected 3 hooks called, got %d", len(order))
	}
	// Reverse order: third(3), second(2), first(1)
	if order[0] != 3 || order[1] != 2 || order[2] != 1 {
		t.Errorf("hooks called in wrong order: %v, expected [3, 2, 1]", order)
	}
}

func TestShutdownManager_TimeoutBehavior(t *testing.T) {
	sm := NewShutdownManager(100*time.Millisecond, discardLogger())

	sm.RegisterHook("slow-hook", func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return nil
		}
	})

	start := time.Now()
	err := sm.Shutdown(context.Background())
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	// Should complete around the deadline, not wait for the full 5s
	if elapsed > 500*time.Millisecond {
		t.Errorf("shutdown took too long: %v", elapsed)
	}
}

func TestShutdownManager_InFlightRequestsCompleteBeforeShutdown(t *testing.T) {
	sm := NewShutdownManager(5*time.Second, discardLogger())

	var requestCompleted atomic.Bool

	// Simulate an in-flight request
	sm.AddInFlight()
	go func() {
		time.Sleep(200 * time.Millisecond)
		requestCompleted.Store(true)
		sm.DoneInFlight()
	}()

	err := sm.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !requestCompleted.Load() {
		t.Error("shutdown did not wait for in-flight request to complete")
	}
}

func TestShutdownManager_InFlightRequestsTimeoutOnDeadline(t *testing.T) {
	sm := NewShutdownManager(100*time.Millisecond, discardLogger())

	// Simulate a request that never completes
	sm.AddInFlight()

	start := time.Now()
	err := sm.Shutdown(context.Background())
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	if elapsed > 500*time.Millisecond {
		t.Errorf("shutdown took too long: %v", elapsed)
	}
}

func TestShutdownManager_SignalTriggersShutdown(t *testing.T) {
	sm := NewShutdownManager(5*time.Second, discardLogger())

	var hookCalled atomic.Bool
	sm.RegisterHook("test-hook", func(ctx context.Context) error {
		hookCalled.Store(true)
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- sm.Run(ctx)
	}()

	// Give Run a moment to set up signal handlers
	time.Sleep(50 * time.Millisecond)

	// Cancel the context to trigger shutdown (simulates signal)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("unexpected error from Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}

	if !hookCalled.Load() {
		t.Error("shutdown hook was not called")
	}
}

func TestShutdownManager_StopAcceptingBeforeDraining(t *testing.T) {
	sm := NewShutdownManager(5*time.Second, discardLogger())

	var sequence []string
	var mu sync.Mutex

	sm.SetStopFunc(func() {
		mu.Lock()
		sequence = append(sequence, "stop-accepting")
		mu.Unlock()
	})

	sm.RegisterHook("hook", func(ctx context.Context) error {
		mu.Lock()
		sequence = append(sequence, "hook")
		mu.Unlock()
		return nil
	})

	err := sm.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(sequence) != 2 {
		t.Fatalf("expected 2 events, got %d: %v", len(sequence), sequence)
	}
	if sequence[0] != "stop-accepting" {
		t.Errorf("expected stop-accepting first, got %q", sequence[0])
	}
	if sequence[1] != "hook" {
		t.Errorf("expected hook second, got %q", sequence[1])
	}
}

func TestShutdownManager_ShutdownOnlyOnce(t *testing.T) {
	sm := NewShutdownManager(5*time.Second, discardLogger())

	var callCount atomic.Int32
	sm.RegisterHook("counter", func(ctx context.Context) error {
		callCount.Add(1)
		return nil
	})

	err1 := sm.Shutdown(context.Background())
	err2 := sm.Shutdown(context.Background())

	if err1 != nil {
		t.Fatalf("first shutdown error: %v", err1)
	}
	if err2 != nil {
		t.Fatalf("second shutdown error: %v", err2)
	}

	if callCount.Load() != 1 {
		t.Errorf("hook called %d times, expected 1", callCount.Load())
	}
}

func TestShutdownManager_NoHooksSucceeds(t *testing.T) {
	sm := NewShutdownManager(5*time.Second, discardLogger())

	err := sm.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestShutdownManager_LoggingMessages(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	sm := NewShutdownManager(5*time.Second, logger)

	sm.SetStopFunc(func() {})
	sm.RegisterHook("test-hook", func(ctx context.Context) error {
		return nil
	})

	var flushCalled atomic.Bool
	sm.SetLogFlush(func() {
		flushCalled.Store(true)
	})

	err := sm.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse JSON log lines
	type logEntry struct {
		Level string `json:"level"`
		Msg   string `json:"msg"`
	}

	var entries []logEntry
	decoder := json.NewDecoder(&buf)
	for decoder.More() {
		var entry logEntry
		if err := decoder.Decode(&entry); err != nil {
			t.Fatalf("failed to decode log entry: %v", err)
		}
		entries = append(entries, entry)
	}

	// Assert exactly 5 INFO messages
	var infoEntries []logEntry
	for _, e := range entries {
		if e.Level == "INFO" {
			infoEntries = append(infoEntries, e)
		}
	}
	if len(infoEntries) != 5 {
		t.Fatalf("expected exactly 5 INFO messages, got %d: %+v", len(infoEntries), infoEntries)
	}

	expectedMsgs := []string{
		"shutdown initiated",
		"stopping new connections",
		"draining in-flight requests",
		"executing shutdown hooks",
		"shutdown complete",
	}
	for i, expected := range expectedMsgs {
		if infoEntries[i].Msg != expected {
			t.Errorf("message %d: expected %q, got %q", i, expected, infoEntries[i].Msg)
		}
	}

	// Assert first and last
	if infoEntries[0].Msg != "shutdown initiated" {
		t.Errorf("first message should be %q, got %q", "shutdown initiated", infoEntries[0].Msg)
	}
	if infoEntries[4].Msg != "shutdown complete" {
		t.Errorf("last message should be %q, got %q", "shutdown complete", infoEntries[4].Msg)
	}

	// Assert logFlush was called
	if !flushCalled.Load() {
		t.Error("logFlush callback was not invoked")
	}
}

func TestShutdownManager_LogFlushNilIsNoop(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	sm := NewShutdownManager(5*time.Second, logger)

	// No SetLogFlush call — logFlush is nil

	err := sm.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should still have 5 log messages and not panic
	type logEntry struct {
		Msg string `json:"msg"`
	}
	var count int
	decoder := json.NewDecoder(&buf)
	for decoder.More() {
		var entry logEntry
		if err := decoder.Decode(&entry); err != nil {
			t.Fatalf("failed to decode log entry: %v", err)
		}
		count++
	}
	if count != 5 {
		t.Fatalf("expected 5 log messages, got %d", count)
	}
}
