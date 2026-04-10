package errors

import (
	"errors"
	"fmt"
	"testing"
)

func TestNixKeyErrorInterface(t *testing.T) {
	err := &ConnectionError{
		NixKeyError: NixKeyError{code: "ERR_CONN_REFUSED", message: "connection refused"},
	}

	if err.Code() != "ERR_CONN_REFUSED" {
		t.Errorf("Code() = %q, want %q", err.Code(), "ERR_CONN_REFUSED")
	}
	if err.Message() != "connection refused" {
		t.Errorf("Message() = %q, want %q", err.Message(), "connection refused")
	}
	if err.Error() != "ERR_CONN_REFUSED: connection refused" {
		t.Errorf("Error() = %q, want %q", err.Error(), "ERR_CONN_REFUSED: connection refused")
	}
}

func TestConnectionError(t *testing.T) {
	err := NewConnectionError("ERR_CONN_REFUSED", "connection refused to 100.64.0.1:29418")
	if err.Code() != "ERR_CONN_REFUSED" {
		t.Errorf("Code() = %q, want %q", err.Code(), "ERR_CONN_REFUSED")
	}

	var connErr *ConnectionError
	if !errors.As(err, &connErr) {
		t.Fatal("errors.As failed for *ConnectionError")
	}

	var nkErr *NixKeyError
	if !errors.As(err, &nkErr) {
		t.Fatal("errors.As failed for *NixKeyError")
	}
}

func TestTimeoutError(t *testing.T) {
	err := NewTimeoutError("ERR_TIMEOUT_SIGN", "sign request timed out after 30s")
	if err.Code() != "ERR_TIMEOUT_SIGN" {
		t.Errorf("Code() = %q, want %q", err.Code(), "ERR_TIMEOUT_SIGN")
	}

	var toErr *TimeoutError
	if !errors.As(err, &toErr) {
		t.Fatal("errors.As failed for *TimeoutError")
	}

	var nkErr *NixKeyError
	if !errors.As(err, &nkErr) {
		t.Fatal("errors.As failed for *NixKeyError")
	}
}

func TestCertError(t *testing.T) {
	err := NewCertError("ERR_CERT_EXPIRED", "certificate expired")
	if err.Code() != "ERR_CERT_EXPIRED" {
		t.Errorf("Code() = %q, want %q", err.Code(), "ERR_CERT_EXPIRED")
	}

	var certErr *CertError
	if !errors.As(err, &certErr) {
		t.Fatal("errors.As failed for *CertError")
	}
}

func TestConfigError(t *testing.T) {
	err := NewConfigError("ERR_CFG_MISSING", "config file not found")
	if err.Code() != "ERR_CFG_MISSING" {
		t.Errorf("Code() = %q, want %q", err.Code(), "ERR_CFG_MISSING")
	}

	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatal("errors.As failed for *ConfigError")
	}
}

func TestProtocolError(t *testing.T) {
	err := NewProtocolError("ERR_PROTO_INVALID", "invalid protobuf message")
	if err.Code() != "ERR_PROTO_INVALID" {
		t.Errorf("Code() = %q, want %q", err.Code(), "ERR_PROTO_INVALID")
	}

	var protoErr *ProtocolError
	if !errors.As(err, &protoErr) {
		t.Fatal("errors.As failed for *ProtocolError")
	}
}

func TestWrapping(t *testing.T) {
	cause := fmt.Errorf("dial tcp 100.64.0.1:29418: connection refused")
	err := NewConnectionError("ERR_CONN_REFUSED", "failed to connect to phone").Wrap(cause)

	// Error message includes cause
	want := "ERR_CONN_REFUSED: failed to connect to phone: dial tcp 100.64.0.1:29418: connection refused"
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}

	// errors.Is works for the cause
	if !errors.Is(err, cause) {
		t.Fatal("errors.Is failed for wrapped cause")
	}

	// Unwrap returns the cause
	if errors.Unwrap(err) != cause {
		t.Fatal("Unwrap did not return cause")
	}

	// errors.As still works
	var connErr *ConnectionError
	if !errors.As(err, &connErr) {
		t.Fatal("errors.As failed for wrapped *ConnectionError")
	}

	var nkErr *NixKeyError
	if !errors.As(err, &nkErr) {
		t.Fatal("errors.As failed for wrapped *NixKeyError")
	}
}

func TestWrappingChain(t *testing.T) {
	root := fmt.Errorf("tls: bad certificate")
	certErr := NewCertError("ERR_CERT_PINNING", "certificate fingerprint mismatch").Wrap(root)
	connErr := NewConnectionError("ERR_CONN_TLS", "mTLS handshake failed").Wrap(certErr)

	// Can extract both error types from the chain
	var extractedCert *CertError
	if !errors.As(connErr, &extractedCert) {
		t.Fatal("errors.As failed for *CertError in chain")
	}
	if extractedCert.Code() != "ERR_CERT_PINNING" {
		t.Errorf("extracted cert error code = %q, want %q", extractedCert.Code(), "ERR_CERT_PINNING")
	}

	var extractedConn *ConnectionError
	if !errors.As(connErr, &extractedConn) {
		t.Fatal("errors.As failed for *ConnectionError in chain")
	}

	// errors.Is reaches the root
	if !errors.Is(connErr, root) {
		t.Fatal("errors.Is failed for root cause in chain")
	}
}

func TestCodeExtraction(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"connection", NewConnectionError("ERR_CONN_REFUSED", "refused"), "ERR_CONN_REFUSED"},
		{"timeout", NewTimeoutError("ERR_TIMEOUT_DIAL", "dial timeout"), "ERR_TIMEOUT_DIAL"},
		{"cert", NewCertError("ERR_CERT_EXPIRED", "expired"), "ERR_CERT_EXPIRED"},
		{"config", NewConfigError("ERR_CFG_INVALID", "invalid"), "ERR_CFG_INVALID"},
		{"protocol", NewProtocolError("ERR_PROTO_VERSION", "version"), "ERR_PROTO_VERSION"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := CodeFrom(tt.err)
			if code != tt.want {
				t.Errorf("CodeFrom() = %q, want %q", code, tt.want)
			}
		})
	}
}

func TestCodeFromNonNixKeyError(t *testing.T) {
	err := fmt.Errorf("some random error")
	code := CodeFrom(err)
	if code != "" {
		t.Errorf("CodeFrom(non-NixKeyError) = %q, want empty string", code)
	}
}

func TestCodeFromWrapped(t *testing.T) {
	inner := NewCertError("ERR_CERT_PINNING", "pin mismatch")
	outer := fmt.Errorf("handshake: %w", inner)

	code := CodeFrom(outer)
	if code != "ERR_CERT_PINNING" {
		t.Errorf("CodeFrom(wrapped) = %q, want %q", code, "ERR_CERT_PINNING")
	}
}

func TestErrorsIsWithSentinels(t *testing.T) {
	err := NewConnectionError("ERR_CONN_REFUSED", "refused")
	if !errors.Is(err, ErrConnection) {
		t.Fatal("errors.Is(connErr, ErrConnection) should be true")
	}
	if errors.Is(err, ErrTimeout) {
		t.Fatal("errors.Is(connErr, ErrTimeout) should be false")
	}

	toErr := NewTimeoutError("ERR_TIMEOUT_SIGN", "sign timeout")
	if !errors.Is(toErr, ErrTimeout) {
		t.Fatal("errors.Is(toErr, ErrTimeout) should be true")
	}

	certErr := NewCertError("ERR_CERT_EXPIRED", "expired")
	if !errors.Is(certErr, ErrCert) {
		t.Fatal("errors.Is(certErr, ErrCert) should be true")
	}

	cfgErr := NewConfigError("ERR_CFG_MISSING", "missing")
	if !errors.Is(cfgErr, ErrConfig) {
		t.Fatal("errors.Is(cfgErr, ErrConfig) should be true")
	}

	protoErr := NewProtocolError("ERR_PROTO_INVALID", "invalid")
	if !errors.Is(protoErr, ErrProtocol) {
		t.Fatal("errors.Is(protoErr, ErrProtocol) should be true")
	}
}
