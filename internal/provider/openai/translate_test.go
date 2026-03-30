package openai

import (
	"encoding/json"
	"testing"

	"github.com/ponchione/sirtopham/internal/provider"
)

func float64Ptr(v float64) *float64 { return &v }

func TestBuildChatRequest_SystemPromptConcatenation(t *testing.T) {
	req := &provider.Request{
		SystemBlocks: []provider.SystemBlock{
			{Text: "You are a coding assistant."},
			{Text: "Always explain your reasoning."},
		},
		Messages: []provider.Message{
			provider.NewUserMessage("hello"),
		},
	}

	cr := buildChatRequest("test-model", req, false)

	if len(cr.Messages) < 1 {
		t.Fatal("expected at least one message")
	}
	sysMsg := cr.Messages[0]
	if sysMsg.Role != "system" {
		t.Fatalf("expected role 'system', got %q", sysMsg.Role)
	}
	expected := "You are a coding assistant.\n\nAlways explain your reasoning."
	if sysMsg.Content != expected {
		t.Fatalf("expected content %q, got %q", expected, sysMsg.Content)
	}
}

func TestBuildChatRequest_EmptySystemBlocks(t *testing.T) {
	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewUserMessage("hello"),
		},
	}

	cr := buildChatRequest("test-model", req, false)

	for _, msg := range cr.Messages {
		if msg.Role == "system" {
			t.Fatal("expected no system message when SystemBlocks is empty")
		}
	}
}

func TestBuildChatRequest_UserMessage(t *testing.T) {
	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewUserMessage("Fix the auth bug"),
		},
	}

	cr := buildChatRequest("test-model", req, false)

	if len(cr.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(cr.Messages))
	}
	msg := cr.Messages[0]
	if msg.Role != "user" {
		t.Fatalf("expected role 'user', got %q", msg.Role)
	}
	if msg.Content != "Fix the auth bug" {
		t.Fatalf("expected content %q, got %q", "Fix the auth bug", msg.Content)
	}
}

func TestBuildChatRequest_AssistantMessageTextOnly(t *testing.T) {
	blocks := []provider.ContentBlock{
		provider.NewTextBlock("I'll check the code."),
	}
	content, _ := json.Marshal(blocks)
	req := &provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleAssistant, Content: content},
		},
	}

	cr := buildChatRequest("test-model", req, false)

	if len(cr.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(cr.Messages))
	}
	msg := cr.Messages[0]
	if msg.Role != "assistant" {
		t.Fatalf("expected role 'assistant', got %q", msg.Role)
	}
	if msg.Content != "I'll check the code." {
		t.Fatalf("expected content %q, got %q", "I'll check the code.", msg.Content)
	}
	if len(msg.ToolCalls) != 0 {
		t.Fatalf("expected no tool_calls, got %d", len(msg.ToolCalls))
	}
}

func TestBuildChatRequest_AssistantMessageWithTextAndToolCalls(t *testing.T) {
	blocks := []provider.ContentBlock{
		provider.NewTextBlock("I found the issue."),
		provider.NewToolUseBlock("call_1", "file_read", json.RawMessage(`{"path":"auth.go"}`)),
	}
	content, _ := json.Marshal(blocks)
	req := &provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleAssistant, Content: content},
		},
	}

	cr := buildChatRequest("test-model", req, false)

	if len(cr.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(cr.Messages))
	}
	msg := cr.Messages[0]
	if msg.Role != "assistant" {
		t.Fatalf("expected role 'assistant', got %q", msg.Role)
	}
	if msg.Content != "I found the issue." {
		t.Fatalf("expected content %q, got %q", "I found the issue.", msg.Content)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(msg.ToolCalls))
	}
	tc := msg.ToolCalls[0]
	if tc.ID != "call_1" {
		t.Errorf("expected tool call ID 'call_1', got %q", tc.ID)
	}
	if tc.Type != "function" {
		t.Errorf("expected tool call type 'function', got %q", tc.Type)
	}
	if tc.Function.Name != "file_read" {
		t.Errorf("expected function name 'file_read', got %q", tc.Function.Name)
	}
	if tc.Function.Arguments != `{"path":"auth.go"}` {
		t.Errorf("expected arguments %q, got %q", `{"path":"auth.go"}`, tc.Function.Arguments)
	}
}

func TestBuildChatRequest_AssistantMessageToolCallsOnly(t *testing.T) {
	blocks := []provider.ContentBlock{
		provider.NewToolUseBlock("call_1", "file_read", json.RawMessage(`{"path":"auth.go"}`)),
	}
	content, _ := json.Marshal(blocks)
	req := &provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleAssistant, Content: content},
		},
	}

	cr := buildChatRequest("test-model", req, false)

	msg := cr.Messages[0]
	if msg.Content != "" {
		t.Fatalf("expected empty content, got %q", msg.Content)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(msg.ToolCalls))
	}

	// Verify empty content is omitted from JSON.
	data, _ := json.Marshal(msg)
	var raw map[string]json.RawMessage
	json.Unmarshal(data, &raw)
	if _, hasContent := raw["content"]; hasContent {
		t.Fatal("expected 'content' to be omitted from JSON when empty")
	}
}

func TestBuildChatRequest_ToolResultMessage(t *testing.T) {
	msg := provider.NewToolResultMessage("call_1", "file_read", "package auth...")
	req := &provider.Request{
		Messages: []provider.Message{msg},
	}

	cr := buildChatRequest("test-model", req, false)

	if len(cr.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(cr.Messages))
	}
	cm := cr.Messages[0]
	if cm.Role != "tool" {
		t.Fatalf("expected role 'tool', got %q", cm.Role)
	}
	if cm.ToolCallID != "call_1" {
		t.Fatalf("expected tool_call_id 'call_1', got %q", cm.ToolCallID)
	}
	if cm.Content != "package auth..." {
		t.Fatalf("expected content %q, got %q", "package auth...", cm.Content)
	}
}

func TestBuildChatRequest_ToolDefinitions(t *testing.T) {
	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewUserMessage("hello"),
		},
		Tools: []provider.ToolDefinition{
			{
				Name:        "file_read",
				Description: "Read a file",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
			},
		},
	}

	cr := buildChatRequest("test-model", req, false)

	if len(cr.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(cr.Tools))
	}
	tool := cr.Tools[0]
	if tool.Type != "function" {
		t.Errorf("expected type 'function', got %q", tool.Type)
	}
	if tool.Function.Name != "file_read" {
		t.Errorf("expected name 'file_read', got %q", tool.Function.Name)
	}
	if tool.Function.Description != "Read a file" {
		t.Errorf("expected description %q, got %q", "Read a file", tool.Function.Description)
	}

	// Verify parameters match.
	var params map[string]interface{}
	if err := json.Unmarshal(tool.Function.Parameters, &params); err != nil {
		t.Fatalf("failed to unmarshal parameters: %v", err)
	}
	if params["type"] != "object" {
		t.Errorf("expected parameters type 'object', got %v", params["type"])
	}
}

func TestBuildChatRequest_NoTools(t *testing.T) {
	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewUserMessage("hello"),
		},
	}

	cr := buildChatRequest("test-model", req, false)

	if cr.Tools != nil {
		t.Fatal("expected nil tools")
	}

	// Verify omitted from JSON.
	data, _ := json.Marshal(cr)
	var raw map[string]json.RawMessage
	json.Unmarshal(data, &raw)
	if _, hasTools := raw["tools"]; hasTools {
		t.Fatal("expected 'tools' key to be absent from JSON")
	}
}

func TestBuildChatRequest_TemperatureAndMaxTokensSet(t *testing.T) {
	req := &provider.Request{
		Messages:    []provider.Message{provider.NewUserMessage("hello")},
		Temperature: float64Ptr(0.7),
		MaxTokens:   8192,
	}

	cr := buildChatRequest("test-model", req, false)

	if cr.Temperature == nil || *cr.Temperature != 0.7 {
		t.Fatalf("expected temperature 0.7, got %v", cr.Temperature)
	}
	if cr.MaxTokens == nil || *cr.MaxTokens != 8192 {
		t.Fatalf("expected max_tokens 8192, got %v", cr.MaxTokens)
	}
}

func TestBuildChatRequest_TemperatureAndMaxTokensNil(t *testing.T) {
	req := &provider.Request{
		Messages: []provider.Message{provider.NewUserMessage("hello")},
	}

	cr := buildChatRequest("test-model", req, false)

	if cr.Temperature != nil {
		t.Fatalf("expected nil temperature, got %v", cr.Temperature)
	}
	if cr.MaxTokens != nil {
		t.Fatalf("expected nil max_tokens, got %v", cr.MaxTokens)
	}

	// Verify omitted from JSON.
	data, _ := json.Marshal(cr)
	var raw map[string]json.RawMessage
	json.Unmarshal(data, &raw)
	if _, has := raw["temperature"]; has {
		t.Fatal("expected 'temperature' to be omitted from JSON")
	}
	if _, has := raw["max_tokens"]; has {
		t.Fatal("expected 'max_tokens' to be omitted from JSON")
	}
}

func TestBuildChatRequest_StreamFlag(t *testing.T) {
	req := &provider.Request{
		Messages: []provider.Message{provider.NewUserMessage("hello")},
	}

	t.Run("stream true", func(t *testing.T) {
		cr := buildChatRequest("test-model", req, true)
		if !cr.Stream {
			t.Fatal("expected stream=true")
		}
	})

	t.Run("stream false", func(t *testing.T) {
		cr := buildChatRequest("test-model", req, false)
		if cr.Stream {
			t.Fatal("expected stream=false")
		}
	})
}

func TestTranslateResponse_TextContentOnly(t *testing.T) {
	resp := &chatResponse{
		ID:     "chatcmpl-1",
		Object: "chat.completion",
		Model:  "test-model",
		Choices: []chatChoice{
			{
				Index: 0,
				Message: chatMessage{
					Role:    "assistant",
					Content: "I found the issue.",
				},
				FinishReason: "stop",
			},
		},
		Usage: chatUsage{
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
		},
	}

	result, err := translateResponse("test", resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
	if result.Content[0].Type != "text" {
		t.Errorf("expected type 'text', got %q", result.Content[0].Type)
	}
	if result.Content[0].Text != "I found the issue." {
		t.Errorf("expected text %q, got %q", "I found the issue.", result.Content[0].Text)
	}
	if result.StopReason != provider.StopReasonEndTurn {
		t.Errorf("expected stop reason %q, got %q", provider.StopReasonEndTurn, result.StopReason)
	}
	if result.Usage.InputTokens != 100 {
		t.Errorf("expected InputTokens 100, got %d", result.Usage.InputTokens)
	}
	if result.Usage.OutputTokens != 50 {
		t.Errorf("expected OutputTokens 50, got %d", result.Usage.OutputTokens)
	}
	if result.Usage.CacheReadTokens != 0 {
		t.Errorf("expected CacheReadTokens 0, got %d", result.Usage.CacheReadTokens)
	}
	if result.Usage.CacheCreationTokens != 0 {
		t.Errorf("expected CacheCreationTokens 0, got %d", result.Usage.CacheCreationTokens)
	}
}

func TestTranslateResponse_ToolCalls(t *testing.T) {
	resp := &chatResponse{
		ID:     "chatcmpl-2",
		Object: "chat.completion",
		Model:  "test-model",
		Choices: []chatChoice{
			{
				Index: 0,
				Message: chatMessage{
					Role: "assistant",
					ToolCalls: []chatToolCall{
						{
							ID:   "call_1",
							Type: "function",
							Function: chatFunctionCall{
								Name:      "file_read",
								Arguments: `{"path":"auth.go"}`,
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
		Usage: chatUsage{PromptTokens: 80, CompletionTokens: 25, TotalTokens: 105},
	}

	result, err := translateResponse("test", resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
	block := result.Content[0]
	if block.Type != "tool_use" {
		t.Fatalf("expected type 'tool_use', got %q", block.Type)
	}
	if block.ID != "call_1" {
		t.Errorf("expected ID 'call_1', got %q", block.ID)
	}
	if block.Name != "file_read" {
		t.Errorf("expected name 'file_read', got %q", block.Name)
	}
	if string(block.Input) != `{"path":"auth.go"}` {
		t.Errorf("expected input %q, got %q", `{"path":"auth.go"}`, string(block.Input))
	}
	if result.StopReason != provider.StopReasonToolUse {
		t.Errorf("expected stop reason %q, got %q", provider.StopReasonToolUse, result.StopReason)
	}
}

func TestTranslateResponse_TextAndToolCalls(t *testing.T) {
	resp := &chatResponse{
		ID:     "chatcmpl-3",
		Object: "chat.completion",
		Model:  "test-model",
		Choices: []chatChoice{
			{
				Index: 0,
				Message: chatMessage{
					Role:    "assistant",
					Content: "I found the issue.",
					ToolCalls: []chatToolCall{
						{
							ID:   "call_1",
							Type: "function",
							Function: chatFunctionCall{
								Name:      "file_read",
								Arguments: `{"path":"auth.go"}`,
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
		Usage: chatUsage{PromptTokens: 80, CompletionTokens: 25, TotalTokens: 105},
	}

	result, err := translateResponse("test", resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(result.Content))
	}
	if result.Content[0].Type != "text" {
		t.Errorf("expected first block type 'text', got %q", result.Content[0].Type)
	}
	if result.Content[1].Type != "tool_use" {
		t.Errorf("expected second block type 'tool_use', got %q", result.Content[1].Type)
	}
}

func TestTranslateResponse_FinishReasonMapping(t *testing.T) {
	tests := []struct {
		reason   string
		expected provider.StopReason
	}{
		{"stop", provider.StopReasonEndTurn},
		{"tool_calls", provider.StopReasonToolUse},
		{"length", provider.StopReasonMaxTokens},
		{"", provider.StopReasonEndTurn},
		{"content_filter", provider.StopReasonEndTurn},
	}

	for _, tt := range tests {
		t.Run("finish_reason_"+tt.reason, func(t *testing.T) {
			resp := &chatResponse{
				Choices: []chatChoice{
					{
						Message:      chatMessage{Content: "test"},
						FinishReason: tt.reason,
					},
				},
			}
			result, err := translateResponse("test", resp)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.StopReason != tt.expected {
				t.Errorf("for finish_reason %q: expected %q, got %q", tt.reason, tt.expected, result.StopReason)
			}
		})
	}
}

func TestTranslateResponse_EmptyChoices(t *testing.T) {
	resp := &chatResponse{
		Choices: []chatChoice{},
	}

	_, err := translateResponse("test", resp)
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
	if expected := "response contained no choices"; !contains(err.Error(), expected) {
		t.Fatalf("expected error to contain %q, got %q", expected, err.Error())
	}
}

func TestBuildChatRequest_FullRoundTrip(t *testing.T) {
	assistantBlocks := []provider.ContentBlock{
		provider.NewTextBlock("I'll check the code."),
		provider.NewToolUseBlock("call_1", "file_read", json.RawMessage(`{"path":"auth.go"}`)),
	}
	assistantContent, _ := json.Marshal(assistantBlocks)

	req := &provider.Request{
		SystemBlocks: []provider.SystemBlock{
			{Text: "You are a helpful assistant."},
		},
		Messages: []provider.Message{
			provider.NewUserMessage("Fix the auth bug"),
			{Role: provider.RoleAssistant, Content: assistantContent},
			provider.NewToolResultMessage("call_1", "file_read", "package auth..."),
		},
		Tools: []provider.ToolDefinition{
			{
				Name:        "file_read",
				Description: "Read a file",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
			},
		},
		Temperature: float64Ptr(0.7),
		MaxTokens:   8192,
	}

	cr := buildChatRequest("qwen2.5-coder-7b", req, false)

	// Marshal and unmarshal to verify round-trip.
	data, err := json.Marshal(cr)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	// Verify model.
	if result["model"] != "qwen2.5-coder-7b" {
		t.Errorf("expected model 'qwen2.5-coder-7b', got %v", result["model"])
	}

	// Verify stream.
	if result["stream"] != false {
		t.Errorf("expected stream false, got %v", result["stream"])
	}

	// Verify temperature.
	if result["temperature"] != 0.7 {
		t.Errorf("expected temperature 0.7, got %v", result["temperature"])
	}

	// Verify max_tokens.
	if result["max_tokens"] != float64(8192) {
		t.Errorf("expected max_tokens 8192, got %v", result["max_tokens"])
	}

	// Verify messages count.
	messages, ok := result["messages"].([]interface{})
	if !ok {
		t.Fatal("expected messages array")
	}
	if len(messages) != 4 { // system + user + assistant + tool
		t.Fatalf("expected 4 messages, got %d", len(messages))
	}

	// Verify system message.
	sysMsg := messages[0].(map[string]interface{})
	if sysMsg["role"] != "system" {
		t.Errorf("expected system role, got %v", sysMsg["role"])
	}
	if sysMsg["content"] != "You are a helpful assistant." {
		t.Errorf("expected system content, got %v", sysMsg["content"])
	}

	// Verify tools.
	tools, ok := result["tools"].([]interface{})
	if !ok {
		t.Fatal("expected tools array")
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
}

func TestSSEChunkTextContent(t *testing.T) {
	payload := `{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`

	var chunk streamChunk
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(chunk.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(chunk.Choices))
	}
	if chunk.Choices[0].Delta.Content != "Hello" {
		t.Errorf("expected content 'Hello', got %q", chunk.Choices[0].Delta.Content)
	}
	if chunk.Choices[0].FinishReason != nil {
		t.Errorf("expected nil finish_reason, got %v", *chunk.Choices[0].FinishReason)
	}
}

func TestSSEToolCallAccumulation(t *testing.T) {
	// Simulate three sequential chunks for a single tool call.
	chunks := []string{
		`{"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"file_read","arguments":""}}]},"finish_reason":null}]}`,
		`{"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":"}}]},"finish_reason":null}]}`,
		`{"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"auth.go\"}"}}]},"finish_reason":null}]}`,
	}

	accumulated := make(map[int]*accumulatedToolCall)

	for _, raw := range chunks {
		var chunk streamChunk
		if err := json.Unmarshal([]byte(raw), &chunk); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}

		for _, tc := range chunk.Choices[0].Delta.ToolCalls {
			acc, exists := accumulated[tc.Index]
			if !exists {
				acc = &accumulatedToolCall{
					ID:   tc.ID,
					Name: tc.Function.Name,
				}
				accumulated[tc.Index] = acc
			}
			acc.Arguments.WriteString(tc.Function.Arguments)
		}
	}

	acc, ok := accumulated[0]
	if !ok {
		t.Fatal("expected accumulated tool call at index 0")
	}
	if acc.ID != "call_1" {
		t.Errorf("expected ID 'call_1', got %q", acc.ID)
	}
	if acc.Name != "file_read" {
		t.Errorf("expected name 'file_read', got %q", acc.Name)
	}
	expected := `{"path":"auth.go"}`
	if acc.Arguments.String() != expected {
		t.Errorf("expected arguments %q, got %q", expected, acc.Arguments.String())
	}
}

func TestSSEFinishReasonDetection(t *testing.T) {
	payload := `{"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`

	var chunk streamChunk
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if chunk.Choices[0].FinishReason == nil {
		t.Fatal("expected non-nil finish_reason")
	}
	if *chunk.Choices[0].FinishReason != "stop" {
		t.Errorf("expected finish_reason 'stop', got %q", *chunk.Choices[0].FinishReason)
	}
}

// contains checks if s contains substr.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
