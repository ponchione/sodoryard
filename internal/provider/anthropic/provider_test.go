package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/provider"
)

// ptr returns a pointer to the given value.
func ptr[T any](v T) *T {
	return &v
}

// mockCredentials implements credentialSource for tests.
type mockCredentials struct {
	headerName  string
	headerValue string
	err         error
}

func (m *mockCredentials) GetAuthHeader(ctx context.Context) (string, string, error) {
	return m.headerName, m.headerValue, m.err
}

func newRetryTestProvider() *AnthropicProvider {
	p := newAnthropicProviderInternal(&mockCredentials{})
	p.sleep = func(context.Context, time.Duration) bool {
		return true
	}
	return p
}

// --- Request Building Tests ---

func TestBuildRequestBody_Defaults(t *testing.T) {
	p := newAnthropicProviderInternal(&mockCredentials{})

	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewUserMessage("hello"),
		},
	}

	body, err := p.buildRequestBody(req, false)
	if err != nil {
		t.Fatalf("buildRequestBody: %v", err)
	}

	if body.Model != "claude-sonnet-4-6-20250514" {
		t.Errorf("Model: expected %q, got %q", "claude-sonnet-4-6-20250514", body.Model)
	}
	if body.MaxTokens != 8192 {
		t.Errorf("MaxTokens: expected 8192, got %d", body.MaxTokens)
	}
	if body.Temperature != nil {
		t.Errorf("Temperature: expected nil, got %v", body.Temperature)
	}
	if body.Stream {
		t.Error("Stream: expected false")
	}
	if body.Thinking != nil {
		t.Error("Thinking: expected nil")
	}
	if body.System != nil {
		t.Error("System: expected nil")
	}
	if body.Tools != nil {
		t.Error("Tools: expected nil")
	}
}

func TestBuildRequestBody_AllFields(t *testing.T) {
	p := newAnthropicProviderInternal(&mockCredentials{})

	req := &provider.Request{
		Model:       "claude-opus-4-6-20250515",
		MaxTokens:   4096,
		Temperature: ptr(0.7),
		SystemBlocks: []provider.SystemBlock{
			{Text: "System prompt 1", CacheControl: &provider.CacheControl{Type: "ephemeral"}},
			{Text: "System prompt 2"},
		},
		Messages: []provider.Message{
			provider.NewUserMessage("Fix the auth bug"),
			{Role: provider.RoleAssistant, Content: json.RawMessage(`[{"type":"text","text":"I'll look into it."}]`)},
		},
		Tools: []provider.ToolDefinition{
			{
				Name:        "file_read",
				Description: "Read a file",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
			},
		},
	}

	body, err := p.buildRequestBody(req, true)
	if err != nil {
		t.Fatalf("buildRequestBody: %v", err)
	}

	if body.Model != "claude-opus-4-6-20250515" {
		t.Errorf("Model: expected %q, got %q", "claude-opus-4-6-20250515", body.Model)
	}
	if body.MaxTokens != 4096 {
		t.Errorf("MaxTokens: expected 4096, got %d", body.MaxTokens)
	}
	if body.Temperature == nil || *body.Temperature != 0.7 {
		t.Errorf("Temperature: expected 0.7, got %v", body.Temperature)
	}
	if !body.Stream {
		t.Error("Stream: expected true")
	}

	// System blocks.
	if len(body.System) != 2 {
		t.Fatalf("System: expected 2 blocks, got %d", len(body.System))
	}
	if body.System[0].CacheControl == nil {
		t.Error("System[0].CacheControl: expected non-nil")
	} else if body.System[0].CacheControl.Type != "ephemeral" {
		t.Errorf("System[0].CacheControl.Type: expected %q, got %q", "ephemeral", body.System[0].CacheControl.Type)
	}
	if body.System[1].CacheControl != nil {
		t.Error("System[1].CacheControl: expected nil")
	}

	// Messages.
	if len(body.Messages) != 2 {
		t.Fatalf("Messages: expected 2, got %d", len(body.Messages))
	}
	if body.Messages[0].Role != "user" {
		t.Errorf("Messages[0].Role: expected %q, got %q", "user", body.Messages[0].Role)
	}

	// Tools.
	if len(body.Tools) != 1 {
		t.Fatalf("Tools: expected 1, got %d", len(body.Tools))
	}
	if body.Tools[0].Name != "file_read" {
		t.Errorf("Tools[0].Name: expected %q, got %q", "file_read", body.Tools[0].Name)
	}
}

func TestBuildRequestBody_ToolMessage(t *testing.T) {
	p := newAnthropicProviderInternal(&mockCredentials{})

	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewToolResultMessage("toolu_1", "file_read", "file contents here"),
		},
	}

	body, err := p.buildRequestBody(req, false)
	if err != nil {
		t.Fatalf("buildRequestBody: %v", err)
	}

	if body.Messages[0].Role != "user" {
		t.Errorf("Role: expected %q, got %q", "user", body.Messages[0].Role)
	}

	// Parse the content to verify the tool_result structure.
	var content []map[string]interface{}
	if err := json.Unmarshal(body.Messages[0].Content, &content); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	if len(content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(content))
	}
	if content[0]["type"] != "tool_result" {
		t.Errorf("type: expected %q, got %v", "tool_result", content[0]["type"])
	}
	if content[0]["tool_use_id"] != "toolu_1" {
		t.Errorf("tool_use_id: expected %q, got %v", "toolu_1", content[0]["tool_use_id"])
	}
}

func TestBuildRequestBody_ThinkingEnabled(t *testing.T) {
	p := newAnthropicProviderInternal(&mockCredentials{})

	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewUserMessage("hello"),
		},
		ProviderOptions: NewAnthropicOptions(true, 5000),
	}

	body, err := p.buildRequestBody(req, false)
	if err != nil {
		t.Fatalf("buildRequestBody: %v", err)
	}

	if body.Thinking == nil {
		t.Fatal("Thinking: expected non-nil")
	}
	if body.Thinking.Type != "enabled" {
		t.Errorf("Thinking.Type: expected %q, got %q", "enabled", body.Thinking.Type)
	}
	if body.Thinking.BudgetTokens != 5000 {
		t.Errorf("Thinking.BudgetTokens: expected 5000, got %d", body.Thinking.BudgetTokens)
	}
}

func TestBuildRequestBody_ThinkingDefaultBudget(t *testing.T) {
	p := newAnthropicProviderInternal(&mockCredentials{})

	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewUserMessage("hello"),
		},
		ProviderOptions: NewAnthropicOptions(true, 0),
	}

	body, err := p.buildRequestBody(req, false)
	if err != nil {
		t.Fatalf("buildRequestBody: %v", err)
	}

	if body.Thinking == nil {
		t.Fatal("Thinking: expected non-nil")
	}
	if body.Thinking.BudgetTokens != DefaultThinkingBudget {
		t.Errorf("Thinking.BudgetTokens: expected %d, got %d", DefaultThinkingBudget, body.Thinking.BudgetTokens)
	}
}

func TestBuildRequestBody_ThinkingDisablesTemperature(t *testing.T) {
	p := newAnthropicProviderInternal(&mockCredentials{})

	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewUserMessage("hello"),
		},
		Temperature:     ptr(0.5),
		ProviderOptions: NewAnthropicOptions(true, 0),
	}

	body, err := p.buildRequestBody(req, false)
	if err != nil {
		t.Fatalf("buildRequestBody: %v", err)
	}

	if body.Temperature != nil {
		t.Errorf("Temperature: expected nil when thinking is enabled, got %v", body.Temperature)
	}
}

// --- HTTP Request Tests ---

func TestBuildHTTPRequest_Headers_OAuth(t *testing.T) {
	creds := &mockCredentials{
		headerName:  "Authorization",
		headerValue: "Bearer test-token",
	}
	p := newAnthropicProviderInternal(creds)

	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewUserMessage("hello"),
		},
	}

	httpReq, err := p.buildHTTPRequest(context.Background(), req, false)
	if err != nil {
		t.Fatalf("buildHTTPRequest: %v", err)
	}

	if got := httpReq.Header.Get("Authorization"); got != "Bearer test-token" {
		t.Errorf("Authorization: expected %q, got %q", "Bearer test-token", got)
	}
	if got := httpReq.Header.Get("anthropic-version"); got != "2023-06-01" {
		t.Errorf("anthropic-version: expected %q, got %q", "2023-06-01", got)
	}
	if got := httpReq.Header.Get("anthropic-beta"); got != "interleaved-thinking-2025-05-14,oauth-2025-04-20" {
		t.Errorf("anthropic-beta: expected %q, got %q", "interleaved-thinking-2025-05-14,oauth-2025-04-20", got)
	}
	if got := httpReq.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type: expected %q, got %q", "application/json", got)
	}
}

func TestBuildHTTPRequest_Headers_APIKey(t *testing.T) {
	creds := &mockCredentials{
		headerName:  "X-Api-Key",
		headerValue: "sk-test-key",
	}
	p := newAnthropicProviderInternal(creds)

	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewUserMessage("hello"),
		},
	}

	httpReq, err := p.buildHTTPRequest(context.Background(), req, false)
	if err != nil {
		t.Fatalf("buildHTTPRequest: %v", err)
	}

	if got := httpReq.Header.Get("X-Api-Key"); got != "sk-test-key" {
		t.Errorf("X-Api-Key: expected %q, got %q", "sk-test-key", got)
	}
	if got := httpReq.Header.Get("Authorization"); got != "" {
		t.Errorf("Authorization: expected empty, got %q", got)
	}
}

func TestBuildHTTPRequest_URL(t *testing.T) {
	creds := &mockCredentials{
		headerName:  "Authorization",
		headerValue: "Bearer test-token",
	}
	p := newAnthropicProviderInternal(creds, WithBaseURL("https://custom.api.com"))

	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewUserMessage("hello"),
		},
	}

	httpReq, err := p.buildHTTPRequest(context.Background(), req, false)
	if err != nil {
		t.Fatalf("buildHTTPRequest: %v", err)
	}

	if httpReq.URL.String() != "https://custom.api.com/v1/messages" {
		t.Errorf("URL: expected %q, got %q", "https://custom.api.com/v1/messages", httpReq.URL.String())
	}
	if httpReq.Method != "POST" {
		t.Errorf("Method: expected POST, got %q", httpReq.Method)
	}
}

func TestBuildHTTPRequest_CredentialError(t *testing.T) {
	creds := &mockCredentials{
		err: errors.New("credential file not found"),
	}
	p := newAnthropicProviderInternal(creds)

	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewUserMessage("hello"),
		},
	}

	_, err := p.buildHTTPRequest(context.Background(), req, false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var pe *provider.ProviderError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *provider.ProviderError, got %T", err)
	}
	if pe.Retriable {
		t.Error("expected Retriable=false")
	}
}

// --- Response Parsing Tests ---

func TestParseResponse_TextOnly(t *testing.T) {
	respJSON := `{
		"id": "msg_1",
		"type": "message",
		"role": "assistant",
		"content": [{"type": "text", "text": "Hello world"}],
		"model": "claude-sonnet-4-6-20250514",
		"stop_reason": "end_turn",
		"usage": {"input_tokens": 10, "output_tokens": 5, "cache_read_input_tokens": 0, "cache_creation_input_tokens": 0}
	}`

	var apiResp apiResponse
	if err := json.Unmarshal([]byte(respJSON), &apiResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Parse content blocks.
	if len(apiResp.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(apiResp.Content))
	}
	if apiResp.Content[0].Type != "text" {
		t.Errorf("content type: expected %q, got %q", "text", apiResp.Content[0].Type)
	}
	if apiResp.Content[0].Text != "Hello world" {
		t.Errorf("content text: expected %q, got %q", "Hello world", apiResp.Content[0].Text)
	}

	// Check stop reason mapping.
	sr := mapStopReason(apiResp.StopReason)
	if sr != provider.StopReasonEndTurn {
		t.Errorf("stop reason: expected %q, got %q", provider.StopReasonEndTurn, sr)
	}

	// Check usage.
	if apiResp.Usage.InputTokens != 10 {
		t.Errorf("input tokens: expected 10, got %d", apiResp.Usage.InputTokens)
	}
	if apiResp.Usage.OutputTokens != 5 {
		t.Errorf("output tokens: expected 5, got %d", apiResp.Usage.OutputTokens)
	}
}

func TestParseResponse_MixedBlocks(t *testing.T) {
	respJSON := `{
		"id": "msg_2",
		"type": "message",
		"role": "assistant",
		"content": [
			{"type": "thinking", "thinking": "Let me analyze..."},
			{"type": "text", "text": "Here is my analysis."},
			{"type": "tool_use", "id": "toolu_1", "name": "file_read", "input": {"path": "main.go"}}
		],
		"model": "claude-sonnet-4-6-20250514",
		"stop_reason": "tool_use",
		"usage": {"input_tokens": 50, "output_tokens": 30, "cache_read_input_tokens": 0, "cache_creation_input_tokens": 0}
	}`

	var apiResp apiResponse
	if err := json.Unmarshal([]byte(respJSON), &apiResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(apiResp.Content) != 3 {
		t.Fatalf("expected 3 content blocks, got %d", len(apiResp.Content))
	}

	if apiResp.Content[0].Type != "thinking" || apiResp.Content[0].Thinking != "Let me analyze..." {
		t.Errorf("block 0: expected thinking block, got type=%q thinking=%q", apiResp.Content[0].Type, apiResp.Content[0].Thinking)
	}
	if apiResp.Content[1].Type != "text" || apiResp.Content[1].Text != "Here is my analysis." {
		t.Errorf("block 1: expected text block, got type=%q text=%q", apiResp.Content[1].Type, apiResp.Content[1].Text)
	}
	if apiResp.Content[2].Type != "tool_use" || apiResp.Content[2].ID != "toolu_1" || apiResp.Content[2].Name != "file_read" {
		t.Errorf("block 2: expected tool_use block, got type=%q id=%q name=%q", apiResp.Content[2].Type, apiResp.Content[2].ID, apiResp.Content[2].Name)
	}
}

func TestParseResponse_ToolUse(t *testing.T) {
	respJSON := `{
		"id": "msg_3",
		"type": "message",
		"role": "assistant",
		"content": [
			{"type": "tool_use", "id": "toolu_1", "name": "file_read", "input": {"path": "auth.go"}}
		],
		"model": "claude-sonnet-4-6-20250514",
		"stop_reason": "tool_use",
		"usage": {"input_tokens": 20, "output_tokens": 10, "cache_read_input_tokens": 0, "cache_creation_input_tokens": 0}
	}`

	var apiResp apiResponse
	if err := json.Unmarshal([]byte(respJSON), &apiResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	block := apiResp.Content[0]
	if block.ID != "toolu_1" {
		t.Errorf("ID: expected %q, got %q", "toolu_1", block.ID)
	}
	if block.Name != "file_read" {
		t.Errorf("Name: expected %q, got %q", "file_read", block.Name)
	}

	var input map[string]string
	if err := json.Unmarshal(block.Input, &input); err != nil {
		t.Fatalf("unmarshal input: %v", err)
	}
	if input["path"] != "auth.go" {
		t.Errorf("Input.path: expected %q, got %q", "auth.go", input["path"])
	}
}

func TestParseResponse_CacheUsage(t *testing.T) {
	respJSON := `{
		"id": "msg_4",
		"type": "message",
		"role": "assistant",
		"content": [{"type": "text", "text": "ok"}],
		"model": "claude-sonnet-4-6-20250514",
		"stop_reason": "end_turn",
		"usage": {"input_tokens": 50, "output_tokens": 5, "cache_read_input_tokens": 1200, "cache_creation_input_tokens": 323}
	}`

	var apiResp apiResponse
	if err := json.Unmarshal([]byte(respJSON), &apiResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if apiResp.Usage.CacheReadInputTokens != 1200 {
		t.Errorf("CacheReadInputTokens: expected 1200, got %d", apiResp.Usage.CacheReadInputTokens)
	}
	if apiResp.Usage.CacheCreationInputTokens != 323 {
		t.Errorf("CacheCreationInputTokens: expected 323, got %d", apiResp.Usage.CacheCreationInputTokens)
	}
}

func TestParseResponse_StopReasons(t *testing.T) {
	tests := []struct {
		input    string
		expected provider.StopReason
	}{
		{"end_turn", provider.StopReasonEndTurn},
		{"tool_use", provider.StopReasonToolUse},
		{"max_tokens", provider.StopReasonMaxTokens},
		{"unknown_value", provider.StopReasonEndTurn},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := mapStopReason(tt.input)
			if got != tt.expected {
				t.Errorf("mapStopReason(%q): expected %q, got %q", tt.input, tt.expected, got)
			}
		})
	}
}

func TestParseResponse_MalformedJSON(t *testing.T) {
	badJSON := []byte(`{this is not valid json`)

	var apiResp apiResponse
	err := json.Unmarshal(badJSON, &apiResp)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	// Verify the error message format matches what Complete would produce.
	msg := "failed to parse response: " + err.Error()
	if !strings.Contains(msg, "failed to parse response") {
		t.Errorf("expected message containing %q, got %q", "failed to parse response", msg)
	}
}

// --- Error Classification Tests ---

func TestClassifyError_StatusCodes(t *testing.T) {
	tests := []struct {
		statusCode      int
		expectedRetry   bool
		expectedContain string
	}{
		{401, false, "authentication failed"},
		{403, false, "authentication failed"},
		{429, true, "rate limit"},
		{400, false, "bad request"},
		{500, true, "internal server error"},
		{502, true, "bad gateway"},
		{503, true, "service unavailable"},
		{418, false, "API error (418)"},
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.statusCode), func(t *testing.T) {
			pe := classifyError(tt.statusCode, []byte("test body"), "")
			if pe.Retriable != tt.expectedRetry {
				t.Errorf("status %d: expected Retriable=%v, got %v", tt.statusCode, tt.expectedRetry, pe.Retriable)
			}
			if !strings.Contains(strings.ToLower(pe.Message), strings.ToLower(tt.expectedContain)) {
				t.Errorf("status %d: expected message containing %q, got %q", tt.statusCode, tt.expectedContain, pe.Message)
			}
			if pe.StatusCode != tt.statusCode {
				t.Errorf("expected StatusCode=%d, got %d", tt.statusCode, pe.StatusCode)
			}
		})
	}
}

func TestClassifyNetworkError(t *testing.T) {
	netErr := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	pe := classifyNetworkError(netErr)

	if !pe.Retriable {
		t.Error("expected Retriable=true")
	}
	if pe.StatusCode != 0 {
		t.Errorf("expected StatusCode=0, got %d", pe.StatusCode)
	}
	if !strings.Contains(pe.Message, "network error") {
		t.Errorf("expected message containing %q, got %q", "network error", pe.Message)
	}
}

// --- Retry Logic Tests ---

func TestDoWithRetry_SuccessFirstAttempt(t *testing.T) {
	p := newRetryTestProvider()
	var callCount int32

	resp, err := p.doWithRetry(context.Background(), func() (*http.Response, error) {
		atomic.AddInt32(&callCount, 1)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected 1 call, got %d", atomic.LoadInt32(&callCount))
	}
}

func TestDoWithRetry_UsesInjectedSleepBetweenRetriableFailures(t *testing.T) {
	p := newAnthropicProviderInternal(&mockCredentials{})
	var delays []time.Duration
	p.sleep = func(_ context.Context, delay time.Duration) bool {
		delays = append(delays, delay)
		return true
	}

	resp, err := p.doWithRetry(context.Background(), func() (*http.Response, error) {
		if len(delays) < 2 {
			return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader("server error"))}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if len(delays) != 2 {
		t.Fatalf("expected 2 injected sleep calls, got %d", len(delays))
	}
}

func TestDoWithRetry_RetryOn429(t *testing.T) {
	p := newRetryTestProvider()
	var callCount int32

	resp, err := p.doWithRetry(context.Background(), func() (*http.Response, error) {
		n := atomic.AddInt32(&callCount, 1)
		if n == 1 {
			return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader("rate limited"))}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if atomic.LoadInt32(&callCount) != 2 {
		t.Errorf("expected 2 calls, got %d", atomic.LoadInt32(&callCount))
	}
}

func TestDoWithRetry_RetryOn500(t *testing.T) {
	p := newRetryTestProvider()
	var callCount int32

	resp, err := p.doWithRetry(context.Background(), func() (*http.Response, error) {
		n := atomic.AddInt32(&callCount, 1)
		if n <= 2 {
			return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader("server error"))}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if atomic.LoadInt32(&callCount) != 3 {
		t.Errorf("expected 3 calls, got %d", atomic.LoadInt32(&callCount))
	}
}

func TestDoWithRetry_NoRetryOn401(t *testing.T) {
	p := newRetryTestProvider()
	var callCount int32

	_, err := p.doWithRetry(context.Background(), func() (*http.Response, error) {
		atomic.AddInt32(&callCount, 1)
		return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader("unauthorized"))}, nil
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var pe *provider.ProviderError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *provider.ProviderError, got %T", err)
	}
	if pe.Retriable {
		t.Error("expected Retriable=false")
	}
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected 1 call, got %d", atomic.LoadInt32(&callCount))
	}
}

func TestDoWithRetry_NoRetryOn400(t *testing.T) {
	p := newRetryTestProvider()
	var callCount int32

	_, err := p.doWithRetry(context.Background(), func() (*http.Response, error) {
		atomic.AddInt32(&callCount, 1)
		return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader("bad request"))}, nil
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected 1 call, got %d", atomic.LoadInt32(&callCount))
	}
}

func TestDoWithRetry_ExhaustedRetries(t *testing.T) {
	p := newRetryTestProvider()
	var callCount int32

	_, err := p.doWithRetry(context.Background(), func() (*http.Response, error) {
		atomic.AddInt32(&callCount, 1)
		return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("unavailable"))}, nil
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var pe *provider.ProviderError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *provider.ProviderError, got %T", err)
	}
	if !pe.Retriable {
		t.Error("expected Retriable=true")
	}
	if atomic.LoadInt32(&callCount) != 3 {
		t.Errorf("expected 3 calls, got %d", atomic.LoadInt32(&callCount))
	}
}

func TestDoWithRetry_ContextCancellation(t *testing.T) {
	p := newAnthropicProviderInternal(&mockCredentials{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Pre-cancel.

	_, err := p.doWithRetry(ctx, func() (*http.Response, error) {
		return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("unavailable"))}, nil
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDoWithRetry_NetworkError(t *testing.T) {
	p := newRetryTestProvider()
	var callCount int32

	_, err := p.doWithRetry(context.Background(), func() (*http.Response, error) {
		n := atomic.AddInt32(&callCount, 1)
		if n <= 2 {
			return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atomic.LoadInt32(&callCount) != 3 {
		t.Errorf("expected 3 calls, got %d", atomic.LoadInt32(&callCount))
	}
}

// --- Models Test ---

func TestModels(t *testing.T) {
	p := newAnthropicProviderInternal(&mockCredentials{})

	models, err := p.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}

	if len(models) != 3 {
		t.Fatalf("expected 3 models, got %d", len(models))
	}

	expectedIDs := []string{
		"claude-sonnet-4-6-20250514",
		"claude-opus-4-6-20250515",
		"claude-haiku-4-5-20251001",
	}

	for i, m := range models {
		if m.ID != expectedIDs[i] {
			t.Errorf("model %d: expected ID %q, got %q", i, expectedIDs[i], m.ID)
		}
		if m.ContextWindow != 200000 {
			t.Errorf("model %d: expected ContextWindow 200000, got %d", i, m.ContextWindow)
		}
		if !m.SupportsTools {
			t.Errorf("model %d: expected SupportsTools=true", i)
		}
		if !m.SupportsThinking {
			t.Errorf("model %d: expected SupportsThinking=true", i)
		}
	}
}
