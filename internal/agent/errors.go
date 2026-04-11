package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ponchione/sodoryard/internal/provider"
)

// Error classification constants for the agent loop's error recovery layer.
const (
	// ErrorCodeRateLimit indicates the provider returned HTTP 429.
	ErrorCodeRateLimit = "rate_limit"
	// ErrorCodeServerError indicates the provider returned HTTP 500/502/503.
	ErrorCodeServerError = "server_error"
	// ErrorCodeAuthFailure indicates the provider returned HTTP 401/403.
	ErrorCodeAuthFailure = "auth_failure"
	// ErrorCodeContextOverflow indicates the request exceeded context length.
	ErrorCodeContextOverflow = "context_overflow"
	// ErrorCodeMalformedToolCall indicates the LLM produced an invalid tool call.
	ErrorCodeMalformedToolCall = "malformed_tool_call"
	// ErrorCodeToolExecution indicates a tool execution failure.
	ErrorCodeToolExecution = "tool_execution"
	// ErrorCodeStreamError indicates a fatal streaming error.
	ErrorCodeStreamError = "stream_error"
	// ErrorCodeUnknown is the fallback for unrecognized errors.
	ErrorCodeUnknown = "unknown_error"
)

// streamErrorClassification holds the results of classifying an error from
// a provider Stream or consumeStream call.
type streamErrorClassification struct {
	// Code is the error classification code (one of the ErrorCode* constants).
	Code string
	// Retriable is true if the error is eligible for automatic retry.
	Retriable bool
	// Message is a human-readable error description with remediation guidance.
	Message string
	// ProviderError is the underlying ProviderError, if the error was one.
	ProviderError *provider.ProviderError
}

// classifyStreamError inspects an error from the provider's Stream or
// consumeStream call and returns a classification for the retry/recovery layer.
func classifyStreamError(err error) streamErrorClassification {
	if err == nil {
		return streamErrorClassification{}
	}

	var pe *provider.ProviderError
	if errors.As(err, &pe) {
		return classifyProviderError(pe)
	}

	// Non-ProviderError: check for context overflow hints in the error message.
	msg := err.Error()
	if isContextOverflowMessage(msg) {
		return streamErrorClassification{
			Code:      ErrorCodeContextOverflow,
			Retriable: false,
			Message:   fmt.Sprintf("Context length exceeded: %s. Emergency compression may recover this.", msg),
		}
	}

	return streamErrorClassification{
		Code:      ErrorCodeUnknown,
		Retriable: false,
		Message:   msg,
	}
}

// classifyProviderError maps a ProviderError to a streamErrorClassification
// based on its HTTP status code.
func classifyProviderError(pe *provider.ProviderError) streamErrorClassification {
	switch {
	case pe.StatusCode == 429:
		return streamErrorClassification{
			Code:          ErrorCodeRateLimit,
			Retriable:     true,
			Message:       fmt.Sprintf("Rate limited by %s. Retrying with backoff.", pe.Provider),
			ProviderError: pe,
		}
	case pe.StatusCode == 500 || pe.StatusCode == 502 || pe.StatusCode == 503:
		return streamErrorClassification{
			Code:          ErrorCodeServerError,
			Retriable:     true,
			Message:       fmt.Sprintf("Server error from %s (HTTP %d). Retrying with backoff.", pe.Provider, pe.StatusCode),
			ProviderError: pe,
		}
	case pe.StatusCode == 401 || pe.StatusCode == 403:
		return streamErrorClassification{
			Code:          ErrorCodeAuthFailure,
			Retriable:     false,
			Message:       fmt.Sprintf("Authentication failed for %s. API key is invalid or expired. Please check your configuration.", pe.Provider),
			ProviderError: pe,
		}
	case pe.StatusCode == 400 && isContextOverflowMessage(pe.Message):
		return streamErrorClassification{
			Code:          ErrorCodeContextOverflow,
			Retriable:     false,
			Message:       fmt.Sprintf("Context length exceeded for %s. Emergency compression may recover this.", pe.Provider),
			ProviderError: pe,
		}
	case pe.Retriable:
		return streamErrorClassification{
			Code:          ErrorCodeServerError,
			Retriable:     true,
			Message:       fmt.Sprintf("Retriable error from %s: %s", pe.Provider, pe.Message),
			ProviderError: pe,
		}
	default:
		return streamErrorClassification{
			Code:          ErrorCodeUnknown,
			Retriable:     false,
			Message:       pe.Error(),
			ProviderError: pe,
		}
	}
}

// isContextOverflowMessage checks if an error message indicates context length exceeded.
func isContextOverflowMessage(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "context_length_exceeded") ||
		strings.Contains(lower, "context length exceeded") ||
		strings.Contains(lower, "maximum context length") ||
		strings.Contains(lower, "token limit exceeded") ||
		strings.Contains(lower, "too many tokens")
}

// toolCallValidationResult holds the outcome of validating a single tool call.
type toolCallValidationResult struct {
	// Valid is true if the tool call parsed correctly.
	Valid bool
	// ErrorMessage is the correction guidance to feed back to the LLM.
	ErrorMessage string
}

type inputSchemaSpec struct {
	Type       string                      `json:"type"`
	Properties map[string]inputSchemaField `json:"properties"`
	Required   []string                    `json:"required"`
}

type inputSchemaField struct {
	Type  string            `json:"type"`
	Enum  []json.RawMessage `json:"enum"`
	Items *inputSchemaField `json:"items"`
}

// validateToolCallJSON checks whether a tool call's Input field is valid JSON.
// If invalid, it returns a descriptive error message suitable for feeding back
// to the LLM as a tool result (so it can self-correct).
func validateToolCallJSON(tc provider.ToolCall) toolCallValidationResult {
	if len(tc.Input) == 0 {
		return toolCallValidationResult{
			Valid:        false,
			ErrorMessage: fmt.Sprintf("Error: tool call '%s' has empty arguments. Please provide valid JSON arguments.", tc.Name),
		}
	}

	// Check if it's valid JSON.
	var parsed any
	if err := json.Unmarshal(tc.Input, &parsed); err != nil {
		return toolCallValidationResult{
			Valid: false,
			ErrorMessage: fmt.Sprintf(
				"Error: invalid JSON in arguments for tool '%s': %s. Ensure all strings are properly quoted and the JSON is well-formed.",
				tc.Name, err.Error(),
			),
		}
	}

	return toolCallValidationResult{Valid: true}
}

func validateToolCallAgainstSchema(tc provider.ToolCall, defs []provider.ToolDefinition) toolCallValidationResult {
	jsonValidation := validateToolCallJSON(tc)
	if !jsonValidation.Valid {
		return jsonValidation
	}
	if len(defs) == 0 {
		return jsonValidation
	}
	def, ok := findToolDefinition(tc.Name, defs)
	if !ok || len(def.InputSchema) == 0 {
		return jsonValidation
	}
	var schema inputSchemaSpec
	if err := json.Unmarshal(def.InputSchema, &schema); err != nil {
		return toolCallValidationResult{
			Valid:        false,
			ErrorMessage: fmt.Sprintf("Error: internal schema for tool '%s' is invalid: %s.", tc.Name, err.Error()),
		}
	}
	if schema.Type != "" && schema.Type != "object" {
		return jsonValidation
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(tc.Input, &payload); err != nil {
		return jsonValidation
	}
	for _, field := range schema.Required {
		value, ok := payload[field]
		if !ok || isJSONNull(value) {
			return toolCallValidationResult{
				Valid:        false,
				ErrorMessage: fmt.Sprintf("Error: tool '%s' is missing required field '%s'. Provide valid JSON arguments matching the declared schema.", tc.Name, field),
			}
		}
	}
	for field, spec := range schema.Properties {
		value, ok := payload[field]
		if !ok {
			continue
		}
		if msg := validateSchemaField(tc.Name, field, value, spec); msg != "" {
			return toolCallValidationResult{Valid: false, ErrorMessage: msg}
		}
	}
	return toolCallValidationResult{Valid: true}
}

func findToolDefinition(name string, defs []provider.ToolDefinition) (provider.ToolDefinition, bool) {
	for _, def := range defs {
		if def.Name == name {
			return def, true
		}
	}
	return provider.ToolDefinition{}, false
}

func validateSchemaField(toolName string, field string, raw json.RawMessage, spec inputSchemaField) string {
	if isJSONNull(raw) {
		return ""
	}
	if spec.Type != "" {
		if ok, expected := matchesSchemaType(raw, spec); !ok {
			return fmt.Sprintf("Error: tool '%s' field '%s' has invalid type; expected %s.", toolName, field, expected)
		}
	}
	if len(spec.Enum) > 0 {
		matched := false
		for _, allowed := range spec.Enum {
			if jsonEqual(raw, allowed) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Sprintf("Error: tool '%s' field '%s' has invalid value. Use one of the allowed values: %s.", toolName, field, formatEnumValues(spec.Enum))
		}
	}
	if spec.Type == "array" && spec.Items != nil {
		var values []json.RawMessage
		if err := json.Unmarshal(raw, &values); err == nil {
			for i, item := range values {
				if msg := validateSchemaField(toolName, fmt.Sprintf("%s[%d]", field, i), item, *spec.Items); msg != "" {
					return msg
				}
			}
		}
	}
	return ""
}

func matchesSchemaType(raw json.RawMessage, spec inputSchemaField) (bool, string) {
	expected := spec.Type
	switch spec.Type {
	case "string":
		var s string
		return json.Unmarshal(raw, &s) == nil, "string"
	case "integer":
		var n float64
		if err := json.Unmarshal(raw, &n); err != nil {
			return false, "integer"
		}
		return n == float64(int64(n)), "integer"
	case "number":
		var n float64
		return json.Unmarshal(raw, &n) == nil, "number"
	case "boolean":
		var b bool
		return json.Unmarshal(raw, &b) == nil, "boolean"
	case "object":
		var obj map[string]json.RawMessage
		return json.Unmarshal(raw, &obj) == nil, "object"
	case "array":
		var arr []json.RawMessage
		return json.Unmarshal(raw, &arr) == nil, "array"
	default:
		return true, expected
	}
}

func isJSONNull(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "null"
}

func jsonEqual(a, b json.RawMessage) bool {
	var left any
	var right any
	if err := json.Unmarshal(a, &left); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &right); err != nil {
		return false
	}
	leftJSON, err := json.Marshal(left)
	if err != nil {
		return false
	}
	rightJSON, err := json.Marshal(right)
	if err != nil {
		return false
	}
	return string(leftJSON) == string(rightJSON)
}

func formatEnumValues(values []json.RawMessage) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strings.TrimSpace(string(value)))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

// enrichToolError adds helpful context to a tool execution error message
// when possible. For example, if a file_read fails with "not found",
// it suggests using list_directory.
func enrichToolError(toolName string, err error) string {
	msg := err.Error()

	switch {
	case toolName == "file_read" && containsAny(msg, "not found", "no such file", "does not exist"):
		return fmt.Sprintf("Error: %s\nHint: The file was not found. Use search_text or list the directory to find the correct path.", msg)

	case toolName == "file_edit" && containsAny(msg, "not found", "no such file", "does not exist"):
		return fmt.Sprintf("Error: %s\nHint: The file was not found. Use file_read to verify the file exists before editing.", msg)

	case toolName == "file_edit" && containsAny(msg, "no match", "not found in file", "search string not found"):
		return fmt.Sprintf("Error: %s\nHint: The search string was not found in the file. Use file_read to check the current file contents.", msg)

	case toolName == "shell" && containsAny(msg, "command not found", "not found"):
		return fmt.Sprintf("Error: %s\nHint: The command was not found. Check the command name and ensure it is installed.", msg)

	case toolName == "shell" && containsAny(msg, "permission denied"):
		return fmt.Sprintf("Error: %s\nHint: Permission denied. The command may require different permissions.", msg)

	default:
		return fmt.Sprintf("Error: %s", msg)
	}
}

// containsAny returns true if s contains any of the substrings (case-insensitive).
func containsAny(s string, substrs ...string) bool {
	lower := strings.ToLower(s)
	for _, sub := range substrs {
		if strings.Contains(lower, strings.ToLower(sub)) {
			return true
		}
	}
	return false
}
