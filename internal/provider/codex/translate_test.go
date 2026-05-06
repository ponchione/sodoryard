package codex

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ponchione/sodoryard/internal/provider"
)

func TestBuildResponsesRequest_SystemPromptConcatenation(t *testing.T) {
	req := &provider.Request{
		SystemBlocks: []provider.SystemBlock{
			{Text: "You are a coding assistant."},
			{Text: "Project context: Go backend"},
		},
	}
	rr := buildResponsesRequest("o3", req, false)

	expected := "You are a coding assistant.\n\nProject context: Go backend"
	if rr.Instructions != expected {
		t.Fatalf("expected instructions %q, got %q", expected, rr.Instructions)
	}
	if len(rr.Input) != 0 {
		t.Fatalf("expected no input items when only system blocks are present, got %d", len(rr.Input))
	}
}

func TestBuildResponsesRequest_EmptySystemBlocks(t *testing.T) {
	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewUserMessage("hello"),
		},
	}
	rr := buildResponsesRequest("o3", req, false)

	if rr.Instructions != "You are a helpful assistant." {
		t.Fatalf("expected default instructions, got %q", rr.Instructions)
	}
	if len(rr.Input) == 0 {
		t.Fatal("expected at least one input item")
	}
	if rr.Input[0].Role == "system" {
		t.Error("no system input item should be emitted when SystemBlocks is nil")
	}
	if rr.Input[0].Role != "user" {
		t.Errorf("expected first item role %q, got %q", "user", rr.Input[0].Role)
	}
}

func TestBuildResponsesRequest_UserMessage(t *testing.T) {
	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewUserMessage("Fix the auth bug"),
		},
	}
	rr := buildResponsesRequest("o3", req, false)

	if len(rr.Input) != 1 {
		t.Fatalf("expected 1 input item, got %d", len(rr.Input))
	}
	item := rr.Input[0]
	if item.Role != "user" {
		t.Fatalf("expected role %q, got %q", "user", item.Role)
	}
	content, ok := item.Content.(string)
	if !ok {
		t.Fatalf("expected string content, got %T", item.Content)
	}
	if content != "Fix the auth bug" {
		t.Errorf("expected content %q, got %q", "Fix the auth bug", content)
	}
}

func TestBuildResponsesRequest_AssistantMessageTextOnly(t *testing.T) {
	blocks := []provider.ContentBlock{
		{Type: "text", Text: "I'll check the code."},
	}
	raw, _ := json.Marshal(blocks)
	req := &provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleAssistant, Content: raw},
		},
	}
	rr := buildResponsesRequest("o3", req, false)

	if len(rr.Input) != 1 {
		t.Fatalf("expected 1 input item, got %d", len(rr.Input))
	}
	item := rr.Input[0]
	if item.Role != "assistant" {
		t.Fatalf("expected role %q, got %q", "assistant", item.Role)
	}

	content, ok := item.Content.(string)
	if !ok {
		t.Fatalf("expected string content, got %T", item.Content)
	}
	if content != "I'll check the code." {
		t.Errorf("expected content %q, got %q", "I'll check the code.", content)
	}
}

func TestBuildResponsesRequest_AssistantMessageWithToolUse(t *testing.T) {
	blocks := []provider.ContentBlock{
		{Type: "text", Text: "Let me read that."},
		{Type: "tool_use", ID: "tc_1", Name: "file_read", Input: json.RawMessage(`{"path":"auth.go"}`)},
	}
	raw, _ := json.Marshal(blocks)
	req := &provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleAssistant, Content: raw},
		},
	}
	rr := buildResponsesRequest("o3", req, false)

	if len(rr.Input) != 2 {
		t.Fatalf("expected 2 input items, got %d", len(rr.Input))
	}

	if rr.Input[0].Role != "assistant" {
		t.Fatalf("expected first item role %q, got %q", "assistant", rr.Input[0].Role)
	}
	content, ok := rr.Input[0].Content.(string)
	if !ok {
		t.Fatalf("expected first item string content, got %T", rr.Input[0].Content)
	}
	if content != "Let me read that." {
		t.Fatalf("expected assistant text %q, got %q", "Let me read that.", content)
	}

	fc := rr.Input[1]
	if fc.Type != "function_call" {
		t.Errorf("expected type %q, got %q", "function_call", fc.Type)
	}
	if fc.CallID != "tc_1" {
		t.Errorf("expected CallID %q, got %q", "tc_1", fc.CallID)
	}
	if fc.Name != "file_read" {
		t.Errorf("expected Name %q, got %q", "file_read", fc.Name)
	}
	if fc.Arguments != `{"path":"auth.go"}` {
		t.Errorf("expected Arguments %q, got %q", `{"path":"auth.go"}`, fc.Arguments)
	}
}

func TestBuildResponsesRequest_ToolResultMessage(t *testing.T) {
	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewToolResultMessage("tc_1", "file_read", "package auth..."),
		},
	}
	rr := buildResponsesRequest("o3", req, false)

	if len(rr.Input) != 1 {
		t.Fatalf("expected 1 input item, got %d", len(rr.Input))
	}
	item := rr.Input[0]
	if item.Type != "function_call_output" {
		t.Fatalf("expected type %q, got %q", "function_call_output", item.Type)
	}
	if item.CallID != "tc_1" {
		t.Errorf("expected CallID %q, got %q", "tc_1", item.CallID)
	}
	if item.Output == nil || *item.Output != "package auth..." {
		t.Errorf("expected Output %q, got %#v", "package auth...", item.Output)
	}
}

func TestBuildResponsesRequest_EmptyToolResultIncludesOutputField(t *testing.T) {
	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewToolResultMessage("tc_empty", "shell", ""),
		},
	}
	rr := buildResponsesRequest("o3", req, false)

	if len(rr.Input) != 1 {
		t.Fatalf("expected 1 input item, got %d", len(rr.Input))
	}
	item := rr.Input[0]
	if item.Type != "function_call_output" {
		t.Fatalf("expected type %q, got %q", "function_call_output", item.Type)
	}
	if item.Output == nil || *item.Output != "" {
		t.Fatalf("expected empty output pointer, got %#v", item.Output)
	}

	data, err := json.Marshal(rr)
	if err != nil {
		t.Fatalf("marshal responses request: %v", err)
	}
	if !json.Valid(data) {
		t.Fatalf("marshaled request is invalid JSON: %s", string(data))
	}
	body := string(data)
	if !strings.Contains(body, `"type":"function_call_output"`) || !strings.Contains(body, `"call_id":"tc_empty"`) || !strings.Contains(body, `"output":""`) {
		t.Fatalf("marshaled request missing required empty output field: %s", string(data))
	}
}

func TestBuildResponsesRequest_ToolDefinitions(t *testing.T) {
	req := &provider.Request{
		Tools: []provider.ToolDefinition{
			{
				Name:        "file_read",
				Description: "Read a file",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
			},
		},
	}
	rr := buildResponsesRequest("o3", req, false)

	if len(rr.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(rr.Tools))
	}
	tool := rr.Tools[0]
	if tool.Type != "function" {
		t.Errorf("expected type %q, got %q", "function", tool.Type)
	}
	if tool.Name != "file_read" {
		t.Errorf("expected name %q, got %q", "file_read", tool.Name)
	}
	if tool.Description != "Read a file" {
		t.Errorf("expected description %q, got %q", "Read a file", tool.Description)
	}

	var params map[string]interface{}
	if err := json.Unmarshal(tool.Parameters, &params); err != nil {
		t.Fatalf("failed to unmarshal parameters: %v", err)
	}
	if params["type"] != "object" {
		t.Errorf("expected parameters.type %q, got %v", "object", params["type"])
	}
}

func TestBuildResponsesRequest_ForcesGPT55AndXHighReasoning(t *testing.T) {
	req := &provider.Request{}
	rr := buildResponsesRequest("o3", req, false)

	if rr.Model != "gpt-5.5" {
		t.Fatalf("expected model %q, got %q", "gpt-5.5", rr.Model)
	}
	if rr.Reasoning == nil {
		t.Fatal("expected reasoning config for forced gpt-5.5 model")
	}
	if rr.Reasoning.Effort != "xhigh" {
		t.Errorf("expected effort %q, got %q", "xhigh", rr.Reasoning.Effort)
	}
	if rr.Reasoning.Summary != "auto" {
		t.Errorf("expected summary %q, got %q", "auto", rr.Reasoning.Summary)
	}
	if len(rr.Include) != 1 || rr.Include[0] != "reasoning.encrypted_content" {
		t.Fatalf("Include = %#v, want encrypted reasoning include", rr.Include)
	}
}

func TestBuildResponsesRequest_ForcesGPT55EvenWhenRequestedModelDiffers(t *testing.T) {
	req := &provider.Request{}
	rr := buildResponsesRequest("gpt-4.1", req, false)

	if rr.Model != "gpt-5.5" {
		t.Fatalf("expected model %q, got %q", "gpt-5.5", rr.Model)
	}
	if rr.Reasoning == nil {
		t.Fatal("expected reasoning config for forced gpt-5.5 model")
	}
	if rr.Reasoning.Effort != "xhigh" {
		t.Errorf("expected effort %q, got %q", "xhigh", rr.Reasoning.Effort)
	}
}

func TestBuildResponsesRequest_ReplaysCodexReasoningBeforeAssistantText(t *testing.T) {
	blocks := []provider.ContentBlock{
		provider.NewCodexReasoningBlock("rs_1", "encrypted-reasoning", []provider.ReasoningSummaryBlock{{Type: "summary_text", Text: "summary"}}),
		provider.NewTextBlock("Final answer."),
	}
	raw, _ := json.Marshal(blocks)
	req := &provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleAssistant, Content: raw},
		},
	}

	rr := buildResponsesRequest("o3", req, false)

	if len(rr.Input) != 2 {
		t.Fatalf("Input count = %d, want 2", len(rr.Input))
	}
	if rr.Input[0].Type != "reasoning" {
		t.Fatalf("first input type = %q, want reasoning", rr.Input[0].Type)
	}
	if rr.Input[0].ID != "rs_1" || rr.Input[0].EncryptedContent != "encrypted-reasoning" {
		t.Fatalf("reasoning input = %+v", rr.Input[0])
	}
	if len(rr.Input[0].Summary) != 1 || rr.Input[0].Summary[0].Text != "summary" {
		t.Fatalf("reasoning summary = %#v", rr.Input[0].Summary)
	}
	if rr.Input[1].Role != "assistant" || rr.Input[1].Content != "Final answer." {
		t.Fatalf("assistant input = %+v", rr.Input[1])
	}
}

func TestBuildResponsesRequest_ReasoningOnlyAssistantAddsFollowingEmptyMessage(t *testing.T) {
	blocks := []provider.ContentBlock{
		provider.NewCodexReasoningBlock("rs_1", "encrypted-reasoning", nil),
	}
	raw, _ := json.Marshal(blocks)
	req := &provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleAssistant, Content: raw},
		},
	}

	rr := buildResponsesRequest("o3", req, false)

	if len(rr.Input) != 2 {
		t.Fatalf("Input count = %d, want reasoning item plus empty assistant message", len(rr.Input))
	}
	if rr.Input[0].Type != "reasoning" {
		t.Fatalf("first input type = %q, want reasoning", rr.Input[0].Type)
	}
	if rr.Input[1].Role != "assistant" || rr.Input[1].Content != "" {
		t.Fatalf("following input = %+v, want empty assistant message", rr.Input[1])
	}
}

func TestBuildResponsesRequest_StreamFlag(t *testing.T) {
	req := &provider.Request{}
	rrStream := buildResponsesRequest("o3", req, true)
	if !rrStream.Stream {
		t.Error("expected stream=true")
	}

	rrNoStream := buildResponsesRequest("o3", req, false)
	if rrNoStream.Stream {
		t.Error("expected stream=false")
	}
}

func TestBuildResponsesRequest_JSONOutput(t *testing.T) {
	req := &provider.Request{
		SystemBlocks: []provider.SystemBlock{
			{Text: "You are helpful."},
		},
		Messages: []provider.Message{
			provider.NewUserMessage("hello"),
		},
	}
	rr := buildResponsesRequest("o3", req, false)

	data, err := json.Marshal(rr)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if raw["model"] != "gpt-5.5" {
		t.Errorf("expected model %q, got %v", "gpt-5.5", raw["model"])
	}
	if raw["instructions"] != "You are helpful." {
		t.Errorf("expected instructions %q, got %v", "You are helpful.", raw["instructions"])
	}
	if raw["stream"] != false {
		t.Errorf("expected stream false, got %v", raw["stream"])
	}
	if raw["store"] != false {
		t.Errorf("expected store false, got %v", raw["store"])
	}

	input, ok := raw["input"].([]interface{})
	if !ok {
		t.Fatalf("expected input array, got %T", raw["input"])
	}
	if len(input) != 1 {
		t.Fatalf("expected 1 input item, got %d", len(input))
	}
}
