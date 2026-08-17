package auth

import (
	"errors"
	"strings"
)

// ErrorCodeRequestScoped identifies failures tied to the current request rather
// than the selected credential.
const ErrorCodeRequestScoped = "request_scoped"

const requestScopedErrorCode = ErrorCodeRequestScoped

// ErrorCodeConnectionLifecycle marks transport/session lifecycle failures that
// must skip credential cooldown without being treated as request-scoped faults.
const ErrorCodeConnectionLifecycle = "connection_lifecycle"

const connectionLifecycleErrorCode = ErrorCodeConnectionLifecycle

const upstreamTimeoutErrorCode = "upstream_timeout"

// ErrorCodeForceCooldown marks failures that must enforce credential cooldown.
const ErrorCodeForceCooldown = "force_cooldown"

const (
	markerProviderEnded   = "Provider ended the request"
	markerUpstreamTimeout = "上游响应超时"
)

// hasUpstreamTimeoutMarker performs a literal case-sensitive check for upstream timeout markers.
func hasUpstreamTimeoutMarker(text string) bool {
	return strings.Contains(text, markerProviderEnded) || strings.Contains(text, markerUpstreamTimeout)
}

// newUpstreamTimeoutError returns a semantic upstream timeout Error.
// The upstream text is intentionally not retained: Error.Message is serialized
// into client responses, and timeout payloads may contain provider-local text.
func newUpstreamTimeoutError(_ string) *Error {
	return &Error{
		Code:       upstreamTimeoutErrorCode,
		Message:    "upstream provider timeout; retry the request",
		Retryable:  true,
		HTTPStatus: 503,
	}
}

// isUpstreamTimeoutError checks if an error represents a semantic upstream timeout failure.
func isUpstreamTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	var authErr *Error
	if errors.As(err, &authErr) && authErr != nil && authErr.Code == upstreamTimeoutErrorCode {
		return true
	}
	return hasUpstreamTimeoutMarker(err.Error())
}

// Error describes an authentication related failure in a provider agnostic format.
type Error struct {
	// Code is a short machine readable identifier.
	Code string `json:"code,omitempty"`
	// Message is a human readable description of the failure.
	Message string `json:"message"`
	// Retryable indicates whether a retry might fix the issue automatically.
	Retryable bool `json:"retryable"`
	// HTTPStatus optionally records an HTTP-like status code for the error.
	HTTPStatus int `json:"http_status,omitempty"`
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

// StatusCode implements optional status accessor for manager decision making.
func (e *Error) StatusCode() int {
	if e == nil {
		return 0
	}
	return e.HTTPStatus
}

// IsRequestScoped reports whether the failure is tied to the current request
// rather than the selected credential.
func (e *Error) IsRequestScoped() bool {
	return e != nil && e.Code == ErrorCodeRequestScoped
}

// MarkRequestScoped marks the error as request-scoped in place and returns it.
func (e *Error) MarkRequestScoped() *Error {
	if e != nil {
		e.Code = ErrorCodeRequestScoped
	}
	return e
}

// NewRequestScopedError creates an Error explicitly flagged as request-scoped so
// that credential cooldown is skipped.
func NewRequestScopedError(message string, httpStatus int) *Error {
	return &Error{
		Code:       ErrorCodeRequestScoped,
		Message:    message,
		HTTPStatus: httpStatus,
	}
}
