package errors

import (
	"errors"
	"fmt"
	"testing"
)

// T-HI-17: Error hierarchy validation — all five error types follow the same
// pattern: Is() matches sentinel, As() extracts *NixKeyError, Wrap() chains,
// CodeFrom() extracts code from any depth, and cross-type Is() returns false.
func TestIntegrationErrorHierarchyValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	type errorFactory struct {
		name     string
		sentinel error
		create   func() error
		code     string
	}

	factories := []errorFactory{
		{"Connection", ErrConnection, func() error {
			return NewConnectionError("ERR_CONN_001", "connection failed")
		}, "ERR_CONN_001"},
		{"Timeout", ErrTimeout, func() error {
			return NewTimeoutError("ERR_TIMEOUT_001", "timed out")
		}, "ERR_TIMEOUT_001"},
		{"Cert", ErrCert, func() error {
			return NewCertError("ERR_CERT_001", "cert invalid")
		}, "ERR_CERT_001"},
		{"Config", ErrConfig, func() error {
			return NewConfigError("ERR_CFG_001", "config bad")
		}, "ERR_CFG_001"},
		{"Protocol", ErrProtocol, func() error {
			return NewProtocolError("ERR_PROTO_001", "proto error")
		}, "ERR_PROTO_001"},
	}

	allSentinels := []error{ErrConnection, ErrTimeout, ErrCert, ErrConfig, ErrProtocol}

	for _, f := range factories {
		t.Run(f.name+"_Is", func(t *testing.T) {
			err := f.create()

			// errors.Is matches own sentinel.
			if !errors.Is(err, f.sentinel) {
				t.Errorf("%s error should Is() match its sentinel", f.name)
			}

			// errors.Is does NOT match other sentinels.
			for _, other := range allSentinels {
				if other == f.sentinel {
					continue
				}
				if errors.Is(err, other) {
					t.Errorf("%s error should NOT Is() match %v", f.name, other)
				}
			}
		})

		t.Run(f.name+"_As_NixKeyError", func(t *testing.T) {
			err := f.create()

			var nkErr *NixKeyError
			if !errors.As(err, &nkErr) {
				t.Fatalf("errors.As(*NixKeyError) failed for %s", f.name)
			}
			if nkErr.Code() != f.code {
				t.Errorf("Code() = %q, want %q", nkErr.Code(), f.code)
			}
		})

		t.Run(f.name+"_CodeFrom", func(t *testing.T) {
			err := f.create()

			code := CodeFrom(err)
			if code != f.code {
				t.Errorf("CodeFrom() = %q, want %q", code, f.code)
			}
		})

		t.Run(f.name+"_ErrorFormat", func(t *testing.T) {
			err := f.create()
			s := err.Error()

			// Format: "code: message"
			if s == "" {
				t.Error("Error() should not be empty")
			}
			if s[:len(f.code)] != f.code {
				t.Errorf("Error() should start with code %q, got %q", f.code, s)
			}
		})
	}

	// Test deep wrapping chain: Config wraps Cert wraps Connection wraps root.
	t.Run("DeepWrappingChain", func(t *testing.T) {
		root := fmt.Errorf("tcp: connection reset")
		connErr := NewConnectionError("ERR_CONN_RESET", "connection reset").Wrap(root)
		certErr := NewCertError("ERR_CERT_VERIFY", "cert verification failed").Wrap(connErr)
		cfgErr := NewConfigError("ERR_CFG_STARTUP", "startup failed").Wrap(certErr)

		// CodeFrom extracts the outermost code.
		code := CodeFrom(cfgErr)
		if code != "ERR_CFG_STARTUP" {
			t.Errorf("CodeFrom(outer) = %q, want ERR_CFG_STARTUP", code)
		}

		// errors.Is reaches the root through the chain.
		if !errors.Is(cfgErr, root) {
			t.Error("should reach root cause through chain")
		}

		// errors.As extracts each type from the chain.
		var extractedCert *CertError
		if !errors.As(cfgErr, &extractedCert) {
			t.Error("should extract *CertError from chain")
		}

		var extractedConn *ConnectionError
		if !errors.As(cfgErr, &extractedConn) {
			t.Error("should extract *ConnectionError from chain")
		}

		// Full error message includes all layers.
		errStr := cfgErr.Error()
		if len(errStr) == 0 {
			t.Error("chained error string should not be empty")
		}
	})

	// Test IsConfigError helper.
	t.Run("IsConfigErrorHelper", func(t *testing.T) {
		cfgErr := NewConfigError("ERR_CFG_BAD", "bad config")
		if !IsConfigError(cfgErr) {
			t.Error("IsConfigError should return true for ConfigError")
		}

		connErr := NewConnectionError("ERR_CONN_BAD", "bad conn")
		if IsConfigError(connErr) {
			t.Error("IsConfigError should return false for ConnectionError")
		}

		wrapped := fmt.Errorf("load: %w", cfgErr)
		if !IsConfigError(wrapped) {
			t.Error("IsConfigError should return true for wrapped ConfigError")
		}
	})

	// Test CodeFrom with non-NixKeyError.
	t.Run("CodeFromPlainError", func(t *testing.T) {
		err := fmt.Errorf("plain error")
		code := CodeFrom(err)
		if code != "" {
			t.Errorf("CodeFrom(plain) should be empty, got %q", code)
		}
	})
}
