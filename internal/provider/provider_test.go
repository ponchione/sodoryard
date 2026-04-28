package provider_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/provider"
)

// Compile-time assertions that all StreamEvent variants satisfy the interface.
var _ provider.StreamEvent = provider.TokenDelta{}
var _ provider.StreamEvent = provider.ThinkingDelta{}
var _ provider.StreamEvent = provider.ToolCallStart{}
var _ provider.StreamEvent = provider.ToolCallDelta{}
var _ provider.StreamEvent = provider.ToolCallEnd{}
var _ provider.StreamEvent = provider.StreamUsage{}
var _ provider.StreamEvent = provider.StreamError{}
var _ provider.StreamEvent = provider.StreamDone{}

// Compile-time assertion that ProviderError satisfies the error interface.
var _ error = (*provider.ProviderError)(nil)

func TestNewUserMessage(t *testing.T) {
	msg := provider.NewUserMessage("hello")
	if msg.Role != provider.RoleUser {
		t.Fatalf("expected role %q, got %q", provider.RoleUser, msg.Role)
	}

	var text string
	if err := json.Unmarshal(msg.Content, &text); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	if text != "hello" {
		t.Fatalf("expected content %q, got %q", "hello", text)
	}
}

func TestContentBlocksFromRaw(t *testing.T) {
	blocks := []provider.ContentBlock{
		provider.NewTextBlock("test"),
		provider.NewToolUseBlock("tc_1", "file_read", json.RawMessage(`{"path":"/tmp"}`)),
	}
	raw, err := json.Marshal(blocks)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	got, err := provider.ContentBlocksFromRaw(raw)
	if err != nil {
		t.Fatalf("ContentBlocksFromRaw: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(got))
	}

	if got[0].Type != "text" || got[0].Text != "test" {
		t.Errorf("block 0: expected text block with text %q, got type=%q text=%q", "test", got[0].Type, got[0].Text)
	}
	if got[1].Type != "tool_use" || got[1].ID != "tc_1" || got[1].Name != "file_read" {
		t.Errorf("block 1: expected tool_use tc_1/file_read, got type=%q id=%q name=%q", got[1].Type, got[1].ID, got[1].Name)
	}
}

func TestProviderErrorRetriable(t *testing.T) {
	retriableCodes := []int{429, 500, 502, 503}
	for _, code := range retriableCodes {
		pe := provider.NewProviderError("test", code, "error", nil)
		if !pe.Retriable {
			t.Errorf("status %d: expected Retriable=true", code)
		}
	}

	nonRetriableCodes := []int{400, 401, 403}
	for _, code := range nonRetriableCodes {
		pe := provider.NewProviderError("test", code, "error", nil)
		if pe.Retriable {
			t.Errorf("status %d: expected Retriable=false", code)
		}
	}

	// Network error: status 0 with non-nil err → retriable
	pe := provider.NewProviderError("test", 0, "connection refused", errors.New("dial tcp"))
	if !pe.Retriable {
		t.Error("network error (status 0, non-nil err): expected Retriable=true")
	}

	// Status 0 with nil err → not retriable
	pe = provider.NewProviderError("test", 0, "unknown", nil)
	if pe.Retriable {
		t.Error("status 0 with nil err: expected Retriable=false")
	}
}

func TestProviderErrorFormat(t *testing.T) {
	pe := provider.NewProviderError("anthropic", 429, "rate limited", nil)
	expected := "anthropic: rate limited (status 429)"
	if pe.Error() != expected {
		t.Errorf("expected %q, got %q", expected, pe.Error())
	}

	pe2 := provider.NewProviderError("openai", 0, "connection refused", errors.New("dial tcp"))
	expected2 := "openai: connection refused"
	if pe2.Error() != expected2 {
		t.Errorf("expected %q, got %q", expected2, pe2.Error())
	}
}

func TestProviderErrorUnwrap(t *testing.T) {
	inner := errors.New("dial tcp: connection refused")
	pe := provider.NewProviderError("test", 0, "network error", inner)
	if !errors.Is(pe, inner) {
		t.Error("errors.Is should find inner error via Unwrap")
	}
}

func TestUsageAdd(t *testing.T) {
	a := provider.Usage{InputTokens: 100, OutputTokens: 50, CacheReadTokens: 10, CacheCreationTokens: 5}
	b := provider.Usage{InputTokens: 200, OutputTokens: 75, CacheReadTokens: 20, CacheCreationTokens: 15}
	sum := a.Add(b)

	if sum.InputTokens != 300 {
		t.Errorf("InputTokens: expected 300, got %d", sum.InputTokens)
	}
	if sum.OutputTokens != 125 {
		t.Errorf("OutputTokens: expected 125, got %d", sum.OutputTokens)
	}
	if sum.CacheReadTokens != 30 {
		t.Errorf("CacheReadTokens: expected 30, got %d", sum.CacheReadTokens)
	}
	if sum.CacheCreationTokens != 20 {
		t.Errorf("CacheCreationTokens: expected 20, got %d", sum.CacheCreationTokens)
	}
}

func TestUsageTotal(t *testing.T) {
	u := provider.Usage{InputTokens: 100, OutputTokens: 50}
	if u.Total() != 150 {
		t.Errorf("Total: expected 150, got %d", u.Total())
	}
}

func TestNewToolResultMessage(t *testing.T) {
	msg := provider.NewToolResultMessage("tc_123", "file_read", "file contents here")
	if msg.Role != provider.RoleTool {
		t.Fatalf("expected role %q, got %q", provider.RoleTool, msg.Role)
	}
	if msg.ToolUseID != "tc_123" {
		t.Fatalf("expected ToolUseID %q, got %q", "tc_123", msg.ToolUseID)
	}
	if msg.ToolName != "file_read" {
		t.Fatalf("expected ToolName %q, got %q", "file_read", msg.ToolName)
	}

	var text string
	if err := json.Unmarshal(msg.Content, &text); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	if text != "file contents here" {
		t.Fatalf("expected content %q, got %q", "file contents here", text)
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal message: %v", err)
	}
	if strings.Contains(string(raw), "details") {
		t.Fatalf("tool result message should not serialize details: %s", raw)
	}
}

func TestNewToolResultDetailsCapsLargePayloads(t *testing.T) {
	raw := provider.NewToolResultDetails("shell", map[string]any{
		"summary":       "large result",
		"returned_size": 10,
		"command":       strings.Repeat("x", provider.ToolResultDetailsMaxSize*2),
	})
	if len(raw) > provider.ToolResultDetailsMaxSize {
		t.Fatalf("details size = %d, want <= %d", len(raw), provider.ToolResultDetailsMaxSize)
	}

	var details map[string]any
	if err := json.Unmarshal(raw, &details); err != nil {
		t.Fatalf("unmarshal details: %v", err)
	}
	if details["kind"] != "shell" || details["summary"] != "large result" {
		t.Fatalf("common envelope not preserved: %#v", details)
	}
	if details["details_truncated"] != true {
		t.Fatalf("details_truncated = %#v, want true", details["details_truncated"])
	}
	if _, ok := details["command"]; ok {
		t.Fatalf("tool-specific command should be dropped from capped details: %#v", details)
	}
}

func TestProviderErrorRetryAfterField(t *testing.T) {
	pe := &provider.ProviderError{
		Provider:   "test",
		StatusCode: 429,
		Message:    "rate limited",
		Retriable:  true,
		RetryAfter: 30 * time.Second,
	}
	if pe.RetryAfter != 30*time.Second {
		t.Errorf("RetryAfter = %v, want 30s", pe.RetryAfter)
	}

	// Zero value when not set.
	pe2 := provider.NewProviderError("test", 429, "rate limited", nil)
	if pe2.RetryAfter != 0 {
		t.Errorf("RetryAfter = %v, want 0 (not set)", pe2.RetryAfter)
	}
}

func TestParseRetryAfter_Seconds(t *testing.T) {
	now := time.Now()
	d := provider.ParseRetryAfter("120", now)
	if d != 120*time.Second {
		t.Errorf("ParseRetryAfter(\"120\") = %v, want 120s", d)
	}
}

func TestParseRetryAfter_FractionalSeconds(t *testing.T) {
	now := time.Now()
	d := provider.ParseRetryAfter("1.5", now)
	if d != 1500*time.Millisecond {
		t.Errorf("ParseRetryAfter(\"1.5\") = %v, want 1.5s", d)
	}
}

func TestParseRetryAfter_HTTPDate(t *testing.T) {
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	future := now.Add(60 * time.Second)
	// HTTP dates use RFC1123 with "GMT" (not "UTC").
	headerVal := future.UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
	d := provider.ParseRetryAfter(headerVal, now)
	// Allow small tolerance for rounding.
	if d < 59*time.Second || d > 61*time.Second {
		t.Errorf("ParseRetryAfter(HTTP-date) = %v, want ~60s", d)
	}
}

func TestParseRetryAfter_Empty(t *testing.T) {
	d := provider.ParseRetryAfter("", time.Now())
	if d != 0 {
		t.Errorf("ParseRetryAfter(\"\") = %v, want 0", d)
	}
}

func TestParseRetryAfter_Invalid(t *testing.T) {
	d := provider.ParseRetryAfter("not-a-number-or-date", time.Now())
	if d != 0 {
		t.Errorf("ParseRetryAfter(invalid) = %v, want 0", d)
	}
}

func TestParseRetryAfter_ZeroSeconds(t *testing.T) {
	d := provider.ParseRetryAfter("0", time.Now())
	if d != 0 {
		t.Errorf("ParseRetryAfter(\"0\") = %v, want 0", d)
	}
}

func TestParseRetryAfter_PastDate(t *testing.T) {
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	past := now.Add(-60 * time.Second)
	headerVal := past.UTC().Format(time.RFC1123)
	d := provider.ParseRetryAfter(headerVal, now)
	if d != 0 {
		t.Errorf("ParseRetryAfter(past date) = %v, want 0", d)
	}
}
