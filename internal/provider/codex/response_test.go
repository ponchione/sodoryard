package codex

import (
	"encoding/json"
	"testing"

	"github.com/ponchione/sodoryard/internal/provider"
)

func TestParseOutputItems_TextOnly(t *testing.T) {
	items := []responsesOutputItem{
		{
			Type: "message",
			ID:   "msg_1",
			Role: "assistant",
			Content: []responsesOutputContent{
				{Type: "output_text", Text: "I found the issue."},
			},
		},
	}

	blocks, stopReason := parseOutputItems(items)

	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].Type != "text" {
		t.Errorf("expected type %q, got %q", "text", blocks[0].Type)
	}
	if blocks[0].Text != "I found the issue." {
		t.Errorf("expected text %q, got %q", "I found the issue.", blocks[0].Text)
	}
	if stopReason != provider.StopReasonEndTurn {
		t.Errorf("expected stop reason %q, got %q", provider.StopReasonEndTurn, stopReason)
	}
}

func TestParseOutputItems_FunctionCall(t *testing.T) {
	items := []responsesOutputItem{
		{
			Type:      "function_call",
			ID:        "fc_1",
			CallID:    "call_1",
			Name:      "file_read",
			Arguments: `{"path":"auth.go"}`,
		},
	}

	blocks, stopReason := parseOutputItems(items)

	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].Type != "tool_use" {
		t.Errorf("expected type %q, got %q", "tool_use", blocks[0].Type)
	}
	if blocks[0].ID != "call_1" {
		t.Errorf("expected ID %q, got %q", "call_1", blocks[0].ID)
	}
	if blocks[0].Name != "file_read" {
		t.Errorf("expected Name %q, got %q", "file_read", blocks[0].Name)
	}
	if string(blocks[0].Input) != `{"path":"auth.go"}` {
		t.Errorf("expected Input %q, got %q", `{"path":"auth.go"}`, string(blocks[0].Input))
	}
	if stopReason != provider.StopReasonToolUse {
		t.Errorf("expected stop reason %q, got %q", provider.StopReasonToolUse, stopReason)
	}
}

func TestParseOutputItems_MixedWithReasoning(t *testing.T) {
	items := []responsesOutputItem{
		{
			Type:             "reasoning",
			ID:               "rs_1",
			EncryptedContent: "base64encrypteddata",
		},
		{
			Type: "message",
			ID:   "msg_1",
			Role: "assistant",
			Content: []responsesOutputContent{
				{Type: "output_text", Text: "Let me read that file."},
			},
		},
		{
			Type:      "function_call",
			ID:        "fc_1",
			CallID:    "call_1",
			Name:      "file_read",
			Arguments: `{"path":"auth.go"}`,
		},
	}

	blocks, stopReason := parseOutputItems(items)

	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(blocks))
	}

	// Block 0: thinking
	if blocks[0].Type != "thinking" {
		t.Errorf("block 0: expected type %q, got %q", "thinking", blocks[0].Type)
	}
	if blocks[0].Thinking != "base64encrypteddata" {
		t.Errorf("block 0: expected thinking %q, got %q", "base64encrypteddata", blocks[0].Thinking)
	}

	// Block 1: text
	if blocks[1].Type != "text" {
		t.Errorf("block 1: expected type %q, got %q", "text", blocks[1].Type)
	}
	if blocks[1].Text != "Let me read that file." {
		t.Errorf("block 1: expected text %q, got %q", "Let me read that file.", blocks[1].Text)
	}

	// Block 2: tool_use
	if blocks[2].Type != "tool_use" {
		t.Errorf("block 2: expected type %q, got %q", "tool_use", blocks[2].Type)
	}
	if blocks[2].ID != "call_1" {
		t.Errorf("block 2: expected ID %q, got %q", "call_1", blocks[2].ID)
	}
	if blocks[2].Name != "file_read" {
		t.Errorf("block 2: expected Name %q, got %q", "file_read", blocks[2].Name)
	}

	if stopReason != provider.StopReasonToolUse {
		t.Errorf("expected stop reason %q, got %q", provider.StopReasonToolUse, stopReason)
	}
}

func TestParseOutputItems_UsageMapping(t *testing.T) {
	usage := responsesUsage{
		InputTokens:  500,
		OutputTokens: 150,
		InputTokensDetails: responsesInputDetails{
			CachedTokens: 100,
		},
		OutputTokensDetails: responsesOutputDetails{
			ReasoningTokens: 80,
		},
	}

	unified := provider.Usage{
		InputTokens:         usage.InputTokens,
		OutputTokens:        usage.OutputTokens,
		CacheReadTokens:     usage.InputTokensDetails.CachedTokens,
		CacheCreationTokens: 0,
	}

	if unified.InputTokens != 500 {
		t.Errorf("expected InputTokens 500, got %d", unified.InputTokens)
	}
	if unified.OutputTokens != 150 {
		t.Errorf("expected OutputTokens 150, got %d", unified.OutputTokens)
	}
	if unified.CacheReadTokens != 100 {
		t.Errorf("expected CacheReadTokens 100, got %d", unified.CacheReadTokens)
	}
	if unified.CacheCreationTokens != 0 {
		t.Errorf("expected CacheCreationTokens 0, got %d", unified.CacheCreationTokens)
	}
}

func TestParseOutputItems_StopReasonNoToolCalls(t *testing.T) {
	items := []responsesOutputItem{
		{
			Type: "message",
			ID:   "msg_1",
			Role: "assistant",
			Content: []responsesOutputContent{
				{Type: "output_text", Text: "All done."},
			},
		},
	}

	_, stopReason := parseOutputItems(items)
	if stopReason != provider.StopReasonEndTurn {
		t.Errorf("expected stop reason %q, got %q", provider.StopReasonEndTurn, stopReason)
	}
}

func TestResponsesResponse_FullRoundTrip(t *testing.T) {
	// Verify that a complete responsesResponse can be marshaled/unmarshaled
	resp := responsesResponse{
		ID:     "resp_123",
		Object: "response",
		Model:  "o3",
		Output: []responsesOutputItem{
			{
				Type: "message",
				ID:   "msg_1",
				Role: "assistant",
				Content: []responsesOutputContent{
					{Type: "output_text", Text: "Hello!"},
				},
			},
		},
		Usage: responsesUsage{
			InputTokens:  100,
			OutputTokens: 20,
			InputTokensDetails: responsesInputDetails{
				CachedTokens: 50,
			},
			OutputTokensDetails: responsesOutputDetails{
				ReasoningTokens: 10,
			},
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var parsed responsesResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if parsed.ID != "resp_123" {
		t.Errorf("expected ID %q, got %q", "resp_123", parsed.ID)
	}
	if parsed.Model != "o3" {
		t.Errorf("expected Model %q, got %q", "o3", parsed.Model)
	}
	if len(parsed.Output) != 1 {
		t.Fatalf("expected 1 output item, got %d", len(parsed.Output))
	}
	if parsed.Usage.InputTokens != 100 {
		t.Errorf("expected InputTokens 100, got %d", parsed.Usage.InputTokens)
	}
}
