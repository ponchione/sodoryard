package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ponchione/sodoryard/internal/provider"
)

// --- classifyStreamError tests ---

func TestClassifyStreamError_Nil(t *testing.T) {
	c := classifyStreamError(nil)
	if c.Code != "" {
		t.Fatalf("Code = %q, want empty", c.Code)
	}
}

func TestClassifyStreamError_RateLimit(t *testing.T) {
	err := &provider.ProviderError{
		Provider:   "anthropic",
		StatusCode: 429,
		Message:    "rate limit exceeded",
		Retriable:  true,
	}
	c := classifyStreamError(err)
	if c.Code != ErrorCodeRateLimit {
		t.Fatalf("Code = %q, want %q", c.Code, ErrorCodeRateLimit)
	}
	if !c.Retriable {
		t.Fatal("Retriable = false, want true")
	}
	if c.ProviderError != err {
		t.Fatal("ProviderError not preserved")
	}
	if !strings.Contains(c.Message, "anthropic") {
		t.Fatalf("Message = %q, want containing anthropic", c.Message)
	}
}

func TestClassifyStreamError_ServerErrors(t *testing.T) {
	for _, code := range []int{500, 502, 503} {
		t.Run(fmt.Sprintf("status_%d", code), func(t *testing.T) {
			err := &provider.ProviderError{
				Provider:   "openai",
				StatusCode: code,
				Message:    "internal error",
				Retriable:  true,
			}
			c := classifyStreamError(err)
			if c.Code != ErrorCodeServerError {
				t.Fatalf("Code = %q, want %q", c.Code, ErrorCodeServerError)
			}
			if !c.Retriable {
				t.Fatal("Retriable = false, want true")
			}
		})
	}
}

func TestClassifyStreamError_AuthFailure(t *testing.T) {
	for _, code := range []int{401, 403} {
		t.Run(fmt.Sprintf("status_%d", code), func(t *testing.T) {
			err := &provider.ProviderError{
				Provider:   "anthropic",
				StatusCode: code,
				Message:    "invalid api key",
				Retriable:  false,
			}
			c := classifyStreamError(err)
			if c.Code != ErrorCodeAuthFailure {
				t.Fatalf("Code = %q, want %q", c.Code, ErrorCodeAuthFailure)
			}
			if c.Retriable {
				t.Fatal("Retriable = true, want false")
			}
			if !strings.Contains(c.Message, "Authentication failed") {
				t.Fatalf("Message = %q, want authentication failure guidance", c.Message)
			}
		})
	}
}

func TestClassifyStreamError_ContextOverflow_ProviderError(t *testing.T) {
	err := &provider.ProviderError{
		Provider:   "anthropic",
		StatusCode: 400,
		Message:    "context_length_exceeded: maximum context length is 200000 tokens",
		Retriable:  false,
	}
	c := classifyStreamError(err)
	if c.Code != ErrorCodeContextOverflow {
		t.Fatalf("Code = %q, want %q", c.Code, ErrorCodeContextOverflow)
	}
	if c.Retriable {
		t.Fatal("Retriable = true, want false")
	}
}

func TestClassifyStreamError_ContextOverflow_PlainError(t *testing.T) {
	err := errors.New("context_length_exceeded")
	c := classifyStreamError(err)
	if c.Code != ErrorCodeContextOverflow {
		t.Fatalf("Code = %q, want %q", c.Code, ErrorCodeContextOverflow)
	}
}

func TestClassifyStreamError_ContextOverflow_Variants(t *testing.T) {
	msgs := []string{
		"maximum context length exceeded",
		"token limit exceeded for this model",
		"too many tokens in the request",
	}
	for _, msg := range msgs {
		t.Run(msg, func(t *testing.T) {
			err := errors.New(msg)
			c := classifyStreamError(err)
			if c.Code != ErrorCodeContextOverflow {
				t.Fatalf("Code = %q, want %q for message %q", c.Code, ErrorCodeContextOverflow, msg)
			}
		})
	}
}

func TestClassifyStreamError_UnknownProviderError(t *testing.T) {
	err := &provider.ProviderError{
		Provider:   "local",
		StatusCode: 418,
		Message:    "I'm a teapot",
		Retriable:  false,
	}
	c := classifyStreamError(err)
	if c.Code != ErrorCodeUnknown {
		t.Fatalf("Code = %q, want %q", c.Code, ErrorCodeUnknown)
	}
	if c.Retriable {
		t.Fatal("Retriable = true, want false")
	}
}

func TestClassifyStreamError_GenericRetriableProviderError(t *testing.T) {
	// A ProviderError with Retriable=true but non-standard status code.
	err := &provider.ProviderError{
		Provider:   "custom",
		StatusCode: 0,
		Message:    "connection refused",
		Retriable:  true,
		Err:        errors.New("dial tcp: connection refused"),
	}
	c := classifyStreamError(err)
	if c.Code != ErrorCodeServerError {
		t.Fatalf("Code = %q, want %q", c.Code, ErrorCodeServerError)
	}
	if !c.Retriable {
		t.Fatal("Retriable = false, want true")
	}
}

func TestClassifyStreamError_PlainError(t *testing.T) {
	err := errors.New("something broke")
	c := classifyStreamError(err)
	if c.Code != ErrorCodeUnknown {
		t.Fatalf("Code = %q, want %q", c.Code, ErrorCodeUnknown)
	}
	if c.Retriable {
		t.Fatal("Retriable = true, want false")
	}
}

// --- validateToolCallJSON tests ---

func TestValidateToolCallJSON_ValidObject(t *testing.T) {
	tc := provider.ToolCall{
		ID:    "tc_1",
		Name:  "file_read",
		Input: json.RawMessage(`{"path": "main.go"}`),
	}
	v := validateToolCallJSON(tc)
	if !v.Valid {
		t.Fatalf("Valid = false, want true; error = %q", v.ErrorMessage)
	}
}

func TestValidateToolCallJSON_EmptyInput(t *testing.T) {
	tc := provider.ToolCall{
		ID:    "tc_2",
		Name:  "git_status",
		Input: json.RawMessage(``),
	}
	v := validateToolCallJSON(tc)
	if v.Valid {
		t.Fatal("Valid = true, want false for empty input")
	}
	if !strings.Contains(v.ErrorMessage, "empty arguments") {
		t.Fatalf("ErrorMessage = %q, want containing 'empty arguments'", v.ErrorMessage)
	}
}

func TestValidateToolCallJSON_InvalidJSON(t *testing.T) {
	tc := provider.ToolCall{
		ID:    "tc_3",
		Name:  "file_edit",
		Input: json.RawMessage(`{invalid json}`),
	}
	v := validateToolCallJSON(tc)
	if v.Valid {
		t.Fatal("Valid = true, want false for invalid JSON")
	}
	if !strings.Contains(v.ErrorMessage, "invalid JSON") {
		t.Fatalf("ErrorMessage = %q, want containing 'invalid JSON'", v.ErrorMessage)
	}
	if !strings.Contains(v.ErrorMessage, "file_edit") {
		t.Fatalf("ErrorMessage = %q, want containing tool name 'file_edit'", v.ErrorMessage)
	}
}

func TestValidateToolCallJSON_NullInput(t *testing.T) {
	// JSON null is valid JSON.
	tc := provider.ToolCall{
		ID:    "tc_4",
		Name:  "git_status",
		Input: json.RawMessage(`null`),
	}
	v := validateToolCallJSON(tc)
	if !v.Valid {
		t.Fatalf("Valid = false, want true for null JSON; error = %q", v.ErrorMessage)
	}
}

func TestValidateToolCallJSON_EmptyObject(t *testing.T) {
	tc := provider.ToolCall{
		ID:    "tc_5",
		Name:  "git_status",
		Input: json.RawMessage(`{}`),
	}
	v := validateToolCallJSON(tc)
	if !v.Valid {
		t.Fatalf("Valid = false, want true for empty object; error = %q", v.ErrorMessage)
	}
}

func TestValidateToolCallAgainstSchema_MissingRequiredField(t *testing.T) {
	tc := provider.ToolCall{
		ID:    "tc_6",
		Name:  "file_read",
		Input: json.RawMessage(`{}`),
	}
	defs := []provider.ToolDefinition{{
		Name: "file_read",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": {"type": "string"}
			},
			"required": ["path"]
		}`),
	}}
	v := validateToolCallAgainstSchema(tc, defs)
	if v.Valid {
		t.Fatal("Valid = true, want false for missing required field")
	}
	if !strings.Contains(v.ErrorMessage, "missing required field") {
		t.Fatalf("ErrorMessage = %q, want missing-required guidance", v.ErrorMessage)
	}
	if !strings.Contains(v.ErrorMessage, "path") {
		t.Fatalf("ErrorMessage = %q, want field name", v.ErrorMessage)
	}
}

func TestValidateToolCallAgainstSchema_WrongType(t *testing.T) {
	tc := provider.ToolCall{
		ID:    "tc_7",
		Name:  "file_read",
		Input: json.RawMessage(`{"path": 123}`),
	}
	defs := []provider.ToolDefinition{{
		Name: "file_read",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": {"type": "string"}
			},
			"required": ["path"]
		}`),
	}}
	v := validateToolCallAgainstSchema(tc, defs)
	if v.Valid {
		t.Fatal("Valid = true, want false for wrong field type")
	}
	if !strings.Contains(v.ErrorMessage, "expected string") {
		t.Fatalf("ErrorMessage = %q, want type guidance", v.ErrorMessage)
	}
}

func TestValidateToolCallAgainstSchema_InvalidEnumValue(t *testing.T) {
	tc := provider.ToolCall{
		ID:    "tc_8",
		Name:  "brain_update",
		Input: json.RawMessage(`{"path":"notes/x.md","operation":"rewrite","content":"hi"}`),
	}
	defs := []provider.ToolDefinition{{
		Name: "brain_update",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": {"type": "string"},
				"operation": {"type": "string", "enum": ["append", "prepend", "replace_section"]},
				"content": {"type": "string"}
			},
			"required": ["path", "operation", "content"]
		}`),
	}}
	v := validateToolCallAgainstSchema(tc, defs)
	if v.Valid {
		t.Fatal("Valid = true, want false for invalid enum value")
	}
	if !strings.Contains(v.ErrorMessage, "allowed values") {
		t.Fatalf("ErrorMessage = %q, want enum guidance", v.ErrorMessage)
	}
}

// --- enrichToolError tests ---

func TestEnrichToolError_FileReadNotFound(t *testing.T) {
	err := errors.New("file not found: internal/auth/handler.go")
	msg := enrichToolError("file_read", err)
	if !strings.Contains(msg, "not found") {
		t.Fatalf("msg = %q, want containing 'not found'", msg)
	}
	if !strings.Contains(msg, "Hint:") {
		t.Fatalf("msg = %q, want containing hint", msg)
	}
}

func TestEnrichToolError_FileEditSearchNotFound(t *testing.T) {
	err := errors.New("search string not found in file")
	msg := enrichToolError("file_edit", err)
	if !strings.Contains(msg, "Hint:") {
		t.Fatalf("msg = %q, want containing hint", msg)
	}
	if !strings.Contains(msg, "file_read") {
		t.Fatalf("msg = %q, want suggesting file_read", msg)
	}
}

func TestEnrichToolError_ShellCommandNotFound(t *testing.T) {
	err := errors.New("bash: foobar: command not found")
	msg := enrichToolError("shell", err)
	if !strings.Contains(msg, "Hint:") {
		t.Fatalf("msg = %q, want containing hint", msg)
	}
}

func TestEnrichToolError_GenericError(t *testing.T) {
	err := errors.New("something unexpected")
	msg := enrichToolError("shell", err)
	if !strings.HasPrefix(msg, "Error: ") {
		t.Fatalf("msg = %q, want starting with 'Error: '", msg)
	}
	if strings.Contains(msg, "Hint:") {
		t.Fatalf("msg = %q, want no hint for generic error", msg)
	}
}

func TestEnrichToolError_ShellPermissionDenied(t *testing.T) {
	err := errors.New("permission denied: /etc/shadow")
	msg := enrichToolError("shell", err)
	if !strings.Contains(msg, "Hint:") {
		t.Fatalf("msg = %q, want containing hint", msg)
	}
	if !strings.Contains(msg, "Permission denied") {
		t.Fatalf("msg = %q, want mentioning permission denied", msg)
	}
}

// --- containsAny tests ---

func TestContainsAny_Match(t *testing.T) {
	if !containsAny("File Not Found", "not found", "missing") {
		t.Fatal("expected true for 'File Not Found' containing 'not found'")
	}
}

func TestContainsAny_NoMatch(t *testing.T) {
	if containsAny("all good", "error", "fail") {
		t.Fatal("expected false for 'all good' with 'error', 'fail'")
	}
}

// --- isContextOverflowMessage tests ---

func TestIsContextOverflowMessage(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"context_length_exceeded", true},
		{"Context Length Exceeded", true},
		{"maximum context length is 200000 tokens", true},
		{"token limit exceeded", true},
		{"too many tokens", true},
		{"something else", false},
		{"rate limit exceeded", false},
	}
	for _, tc := range cases {
		t.Run(tc.msg, func(t *testing.T) {
			got := isContextOverflowMessage(tc.msg)
			if got != tc.want {
				t.Fatalf("isContextOverflowMessage(%q) = %v, want %v", tc.msg, got, tc.want)
			}
		})
	}
}
