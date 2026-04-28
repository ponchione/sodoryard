package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// Model describes an LLM model's capabilities. Returned by Provider.Models().
type Model struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Provider         string `json:"provider,omitempty"`
	ContextWindow    int    `json:"context_window"`
	SupportsTools    bool   `json:"supports_tools"`
	SupportsThinking bool   `json:"supports_thinking"`
}

// ToolCall represents a tool invocation requested by the model.
type ToolCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// ToolResult is the response to a ToolCall.
type ToolResult struct {
	ToolUseID string          `json:"tool_use_id"`
	Content   string          `json:"content"`
	IsError   bool            `json:"is_error,omitempty"`
	Details   json.RawMessage `json:"details,omitempty"`
}

// ProviderError is a structured error type for provider failures. It carries
// the HTTP status code, retry eligibility, and originating provider name.
type ProviderError struct {
	Provider    string
	StatusCode  int
	Message     string
	Retriable   bool
	Err         error
	AuthKind    AuthErrorKind
	Remediation string
	// RetryAfter is the server-suggested retry delay parsed from the Retry-After HTTP header.
	RetryAfter time.Duration
}

// Error implements the error interface.
func (e *ProviderError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("%s: %s (status %d)", e.Provider, e.Message, e.StatusCode)
	}
	return fmt.Sprintf("%s: %s", e.Provider, e.Message)
}

// Unwrap returns the underlying error for errors.Is/errors.As chain traversal.
func (e *ProviderError) Unwrap() error {
	return e.Err
}

// ParseRetryAfter parses a Retry-After header value into a time.Duration.
// The header can be either a number of seconds (e.g. "120") or an HTTP-date
// (e.g. "Fri, 31 Dec 1999 23:59:59 GMT"). Returns 0 if the header is empty
// or unparseable.
func ParseRetryAfter(headerVal string, now time.Time) time.Duration {
	if headerVal == "" {
		return 0
	}
	// Try as seconds first.
	if seconds, err := strconv.ParseFloat(headerVal, 64); err == nil {
		if seconds > 0 {
			return time.Duration(seconds * float64(time.Second))
		}
		return 0
	}
	// Try as HTTP-date.
	if t, err := http.ParseTime(headerVal); err == nil {
		d := t.Sub(now)
		if d > 0 {
			return d
		}
		return 0
	}
	return 0
}

// NewProviderError creates a ProviderError with automatic Retriable determination.
// Retriable is true for status codes 429, 500, 502, 503, or when statusCode is 0
// and err is non-nil (network error). False otherwise.
func NewProviderError(provider string, statusCode int, message string, err error) *ProviderError {
	retriable := false
	switch statusCode {
	case 429, 500, 502, 503:
		retriable = true
	case 0:
		retriable = err != nil
	}
	return &ProviderError{
		Provider:   provider,
		StatusCode: statusCode,
		Message:    message,
		Retriable:  retriable,
		Err:        err,
	}
}

// NewAuthProviderError creates a non-retriable ProviderError tagged as an
// authentication failure with provider-specific remediation guidance.
func NewAuthProviderError(provider string, kind AuthErrorKind, statusCode int, message, remediation string, err error) *ProviderError {
	return &ProviderError{
		Provider:    provider,
		StatusCode:  statusCode,
		Message:     message,
		Retriable:   false,
		Err:         err,
		AuthKind:    kind,
		Remediation: remediation,
	}
}

// IsAuthenticationFailure reports whether err is a provider auth failure.
func IsAuthenticationFailure(err error) bool {
	var pe *ProviderError
	if !errors.As(err, &pe) {
		return false
	}
	return pe.AuthKind != "" || pe.StatusCode == http.StatusUnauthorized || pe.StatusCode == http.StatusForbidden
}
