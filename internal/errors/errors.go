// Package errors provides a structured error hierarchy for nix-key.
//
// All error types embed NixKeyError which provides Code() and Message() methods.
// Each error category has a unique code prefix (ERR_CONN_*, ERR_TIMEOUT_*, etc.)
// and supports Go errors.Is() and errors.As() for wrapping/unwrapping.
package errors

import (
	"errors"
	"fmt"
)

// Sentinel errors for use with errors.Is().
var (
	ErrConnection = errors.New("connection error")
	ErrTimeout    = errors.New("timeout error")
	ErrCert       = errors.New("certificate error")
	ErrConfig     = errors.New("config error")
	ErrProtocol   = errors.New("protocol error")
)

// NixKeyError is the base error type for all nix-key errors.
type NixKeyError struct {
	code    string
	message string
	cause   error
}

func (e *NixKeyError) Code() string    { return e.code }
func (e *NixKeyError) Message() string { return e.message }

func (e *NixKeyError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s: %s", e.code, e.message, e.cause.Error())
	}
	return fmt.Sprintf("%s: %s", e.code, e.message)
}

func (e *NixKeyError) Unwrap() error { return e.cause }

// ConnectionError represents network/connection failures.
type ConnectionError struct {
	NixKeyError
}

func NewConnectionError(code, message string) *ConnectionError {
	return &ConnectionError{NixKeyError: NixKeyError{code: code, message: message}}
}

func (e *ConnectionError) Wrap(cause error) *ConnectionError {
	e.cause = cause
	return e
}

func (e *ConnectionError) Is(target error) bool {
	return target == ErrConnection
}

func (e *ConnectionError) As(target interface{}) bool {
	if nk, ok := target.(**NixKeyError); ok {
		*nk = &e.NixKeyError
		return true
	}
	return false
}

// TimeoutError represents timeout failures.
type TimeoutError struct {
	NixKeyError
}

func NewTimeoutError(code, message string) *TimeoutError {
	return &TimeoutError{NixKeyError: NixKeyError{code: code, message: message}}
}

func (e *TimeoutError) Wrap(cause error) *TimeoutError {
	e.cause = cause
	return e
}

func (e *TimeoutError) Is(target error) bool {
	return target == ErrTimeout
}

func (e *TimeoutError) As(target interface{}) bool {
	if nk, ok := target.(**NixKeyError); ok {
		*nk = &e.NixKeyError
		return true
	}
	return false
}

// CertError represents certificate-related failures.
type CertError struct {
	NixKeyError
}

func NewCertError(code, message string) *CertError {
	return &CertError{NixKeyError: NixKeyError{code: code, message: message}}
}

func (e *CertError) Wrap(cause error) *CertError {
	e.cause = cause
	return e
}

func (e *CertError) Is(target error) bool {
	return target == ErrCert
}

func (e *CertError) As(target interface{}) bool {
	if nk, ok := target.(**NixKeyError); ok {
		*nk = &e.NixKeyError
		return true
	}
	return false
}

// ConfigError represents configuration failures.
type ConfigError struct {
	NixKeyError
}

func NewConfigError(code, message string) *ConfigError {
	return &ConfigError{NixKeyError: NixKeyError{code: code, message: message}}
}

func (e *ConfigError) Wrap(cause error) *ConfigError {
	e.cause = cause
	return e
}

func (e *ConfigError) Is(target error) bool {
	return target == ErrConfig
}

func (e *ConfigError) As(target interface{}) bool {
	if nk, ok := target.(**NixKeyError); ok {
		*nk = &e.NixKeyError
		return true
	}
	return false
}

// ProtocolError represents gRPC/protobuf protocol failures.
type ProtocolError struct {
	NixKeyError
}

func NewProtocolError(code, message string) *ProtocolError {
	return &ProtocolError{NixKeyError: NixKeyError{code: code, message: message}}
}

func (e *ProtocolError) Wrap(cause error) *ProtocolError {
	e.cause = cause
	return e
}

func (e *ProtocolError) Is(target error) bool {
	return target == ErrProtocol
}

func (e *ProtocolError) As(target interface{}) bool {
	if nk, ok := target.(**NixKeyError); ok {
		*nk = &e.NixKeyError
		return true
	}
	return false
}

// IsConfigError reports whether err or any error in its chain is a ConfigError.
func IsConfigError(err error) bool {
	return errors.Is(err, ErrConfig)
}

// CodeFrom extracts the error code from any error in the chain that
// implements the Code() string method. Returns empty string if not found.
func CodeFrom(err error) string {
	type coder interface {
		Code() string
	}
	var c coder
	if errors.As(err, &c) {
		return c.Code()
	}
	return ""
}
