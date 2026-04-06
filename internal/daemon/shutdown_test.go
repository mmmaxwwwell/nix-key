package daemon

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestShutdownManager_HooksCalledInReverseOrder(t *testing.T) {
	sm := NewShutdownManager(30 * time.Second)

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
	sm := NewShutdownManager(100 * time.Millisecond)

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
	sm := NewShutdownManager(5 * time.Second)

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
	sm := NewShutdownManager(100 * time.Millisecond)

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
	sm := NewShutdownManager(5 * time.Second)

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
	sm := NewShutdownManager(5 * time.Second)

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
	sm := NewShutdownManager(5 * time.Second)

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
	sm := NewShutdownManager(5 * time.Second)

	err := sm.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
