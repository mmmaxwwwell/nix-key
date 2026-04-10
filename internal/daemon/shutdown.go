package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// ShutdownHook is a function called during shutdown. It receives a context
// that is cancelled when the shutdown deadline expires.
type ShutdownHook func(ctx context.Context) error

type namedHook struct {
	name string
	fn   ShutdownHook
}

// ShutdownManager coordinates graceful shutdown with a hook registry.
type ShutdownManager struct {
	deadline time.Duration
	hooks    []namedHook
	stopFunc func() // called to stop accepting new connections
	logger   *slog.Logger
	logFlush func() // called as the very last step after shutdown complete

	inFlight sync.WaitGroup

	once     sync.Once
	mu       sync.Mutex
	shutdown bool
}

// NewShutdownManager creates a ShutdownManager with the given drain deadline.
func NewShutdownManager(deadline time.Duration, logger *slog.Logger) *ShutdownManager {
	return &ShutdownManager{
		deadline: deadline,
		logger:   logger,
	}
}

// RegisterHook adds a named shutdown hook. Hooks are called in reverse
// registration order during shutdown.
func (sm *ShutdownManager) RegisterHook(name string, fn ShutdownHook) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.hooks = append(sm.hooks, namedHook{name: name, fn: fn})
}

// SetLogFlush sets the function called as the very last step after shutdown
// completes. It can be used to flush buffered log output.
func (sm *ShutdownManager) SetLogFlush(fn func()) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.logFlush = fn
}

// SetStopFunc sets the function called to stop accepting new connections.
// It is called before draining in-flight requests.
func (sm *ShutdownManager) SetStopFunc(fn func()) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.stopFunc = fn
}

// AddInFlight increments the in-flight request counter.
// Call DoneInFlight when the request completes.
func (sm *ShutdownManager) AddInFlight() {
	sm.inFlight.Add(1)
}

// DoneInFlight decrements the in-flight request counter.
func (sm *ShutdownManager) DoneInFlight() {
	sm.inFlight.Done()
}

// Run listens for SIGTERM/SIGINT (or context cancellation) and triggers
// graceful shutdown. It blocks until shutdown is complete.
func (sm *ShutdownManager) Run(ctx context.Context) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigCh)

	select {
	case <-sigCh:
	case <-ctx.Done():
	}

	return sm.Shutdown(context.Background())
}

// Shutdown performs the graceful shutdown sequence. It is safe to call
// multiple times; only the first call executes the sequence.
func (sm *ShutdownManager) Shutdown(ctx context.Context) error {
	var shutdownErr error

	sm.once.Do(func() {
		sm.logger.Info("shutdown initiated")

		sm.mu.Lock()
		sm.shutdown = true
		stopFn := sm.stopFunc
		// Copy hooks under lock
		hooks := make([]namedHook, len(sm.hooks))
		copy(hooks, sm.hooks)
		sm.mu.Unlock()

		// Create a context with the drain deadline
		deadlineCtx, cancel := context.WithTimeout(ctx, sm.deadline)
		defer cancel()

		// Step 1: Stop accepting new connections
		sm.logger.Info("stopping new connections")
		if stopFn != nil {
			stopFn()
		}

		// Step 2: Drain in-flight requests
		sm.logger.Info("draining in-flight requests")
		waitDone := make(chan struct{})
		go func() {
			sm.inFlight.Wait()
			close(waitDone)
		}()

		select {
		case <-waitDone:
			// All in-flight drained
		case <-deadlineCtx.Done():
			shutdownErr = fmt.Errorf("shutdown: timed out waiting for in-flight requests: %w", deadlineCtx.Err())
			return
		}

		// Step 3: Call hooks in reverse order
		sm.logger.Info("executing shutdown hooks")
		var hookErrors []error
		for i := len(hooks) - 1; i >= 0; i-- {
			h := hooks[i]
			if err := h.fn(deadlineCtx); err != nil {
				hookErrors = append(hookErrors, fmt.Errorf("hook %q: %w", h.name, err))
			}
		}

		if len(hookErrors) > 0 {
			shutdownErr = errors.Join(hookErrors...)
		}

		sm.logger.Info("shutdown complete")

		if sm.logFlush != nil {
			sm.logFlush()
		}
	})

	return shutdownErr
}
