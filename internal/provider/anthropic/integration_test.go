package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/provider"
)

// newMockServer creates a test server with a mock credential manager and
// returns both the server and a configured AnthropicProvider.
func newMockServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *AnthropicProvider) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	creds := &mockCredentials{
		headerName:  "Authorization",
		headerValue: "Bearer test-token",
	}

	p := newAnthropicProviderInternal(creds, WithBaseURL(server.URL), WithHTTPClient(&http.Client{Timeout: 10 * time.Second}))
	return server, p
}

// writeSSE writes an SSE event to a ResponseWriter and flushes.
func writeSSE(w http.ResponseWriter, event string, data string) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// --- Complete Method Integration Tests ---

func TestIntegration_Complete_TextResponse(t *testing.T) {
	_, p := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Validate request.
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/messages" {
			t.Errorf("expected /v1/messages, got %s", r.URL.Path)
		}
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Errorf("anthropic-version: expected %q, got %q", "2023-06-01", got)
		}
		beta := r.Header.Get("anthropic-beta")
		if beta != "interleaved-thinking-2025-05-14,oauth-2025-04-20" {
			t.Errorf("anthropic-beta: expected %q, got %q", "interleaved-thinking-2025-05-14,oauth-2025-04-20", beta)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type: expected %q, got %q", "application/json", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization: expected %q, got %q", "Bearer test-token", got)
		}

		// Validate request body.
		body, _ := io.ReadAll(r.Body)
		var reqBody map[string]interface{}
		json.Unmarshal(body, &reqBody)
		if reqBody["stream"] != false {
			t.Errorf("stream: expected false, got %v", reqBody["stream"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{
			"id": "msg_test1",
			"type": "message",
			"role": "assistant",
			"content": [{"type": "text", "text": "The answer is 42."}],
			"model": "claude-sonnet-4-6-20250514",
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 50, "output_tokens": 12, "cache_read_input_tokens": 30, "cache_creation_input_tokens": 20}
		}`)
	})

	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewUserMessage("What is the answer?"),
		},
	}

	resp, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(resp.Content))
	}
	if resp.Content[0].Type != "text" || resp.Content[0].Text != "The answer is 42." {
		t.Errorf("content: expected text %q, got type=%q text=%q", "The answer is 42.", resp.Content[0].Type, resp.Content[0].Text)
	}
	if resp.StopReason != provider.StopReasonEndTurn {
		t.Errorf("stop reason: expected %q, got %q", provider.StopReasonEndTurn, resp.StopReason)
	}
	if resp.Model != "claude-sonnet-4-6-20250514" {
		t.Errorf("model: expected %q, got %q", "claude-sonnet-4-6-20250514", resp.Model)
	}
	if resp.Usage.InputTokens != 50 {
		t.Errorf("input tokens: expected 50, got %d", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 12 {
		t.Errorf("output tokens: expected 12, got %d", resp.Usage.OutputTokens)
	}
	if resp.Usage.CacheReadTokens != 30 {
		t.Errorf("cache read tokens: expected 30, got %d", resp.Usage.CacheReadTokens)
	}
	if resp.Usage.CacheCreationTokens != 20 {
		t.Errorf("cache creation tokens: expected 20, got %d", resp.Usage.CacheCreationTokens)
	}
	if resp.LatencyMs < 0 {
		t.Errorf("latency: expected >= 0, got %d", resp.LatencyMs)
	}
}

func TestIntegration_Complete_ToolUseResponse(t *testing.T) {
	_, p := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{
			"id": "msg_test2",
			"type": "message",
			"role": "assistant",
			"content": [
				{"type": "text", "text": "Let me read that file."},
				{"type": "tool_use", "id": "toolu_abc", "name": "file_read", "input": {"path": "main.go"}}
			],
			"model": "claude-sonnet-4-6-20250514",
			"stop_reason": "tool_use",
			"usage": {"input_tokens": 100, "output_tokens": 45, "cache_read_input_tokens": 0, "cache_creation_input_tokens": 0}
		}`)
	})

	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewUserMessage("Read main.go"),
		},
	}

	resp, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if len(resp.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(resp.Content))
	}
	if resp.Content[0].Type != "text" {
		t.Errorf("block 0: expected type %q, got %q", "text", resp.Content[0].Type)
	}
	if resp.Content[1].Type != "tool_use" {
		t.Errorf("block 1: expected type %q, got %q", "tool_use", resp.Content[1].Type)
	}
	if resp.Content[1].ID != "toolu_abc" {
		t.Errorf("block 1 ID: expected %q, got %q", "toolu_abc", resp.Content[1].ID)
	}
	if resp.Content[1].Name != "file_read" {
		t.Errorf("block 1 Name: expected %q, got %q", "file_read", resp.Content[1].Name)
	}

	var input map[string]string
	if err := json.Unmarshal(resp.Content[1].Input, &input); err != nil {
		t.Fatalf("unmarshal input: %v", err)
	}
	if input["path"] != "main.go" {
		t.Errorf("input path: expected %q, got %q", "main.go", input["path"])
	}
	if resp.StopReason != provider.StopReasonToolUse {
		t.Errorf("stop reason: expected %q, got %q", provider.StopReasonToolUse, resp.StopReason)
	}
}

func TestIntegration_Complete_ThinkingResponse(t *testing.T) {
	_, p := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{
			"id": "msg_test3",
			"type": "message",
			"role": "assistant",
			"content": [
				{"type": "thinking", "thinking": "Let me analyze the code..."},
				{"type": "text", "text": "I found the issue in auth.go."}
			],
			"model": "claude-sonnet-4-6-20250514",
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 80, "output_tokens": 30, "cache_read_input_tokens": 0, "cache_creation_input_tokens": 0}
		}`)
	})

	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewUserMessage("Find the bug"),
		},
	}

	resp, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if len(resp.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(resp.Content))
	}
	if resp.Content[0].Type != "thinking" || resp.Content[0].Thinking != "Let me analyze the code..." {
		t.Errorf("block 0: expected thinking %q, got type=%q thinking=%q", "Let me analyze the code...", resp.Content[0].Type, resp.Content[0].Thinking)
	}
	if resp.Content[1].Type != "text" || resp.Content[1].Text != "I found the issue in auth.go." {
		t.Errorf("block 1: expected text %q, got type=%q text=%q", "I found the issue in auth.go.", resp.Content[1].Type, resp.Content[1].Text)
	}
}

func TestIntegration_Complete_RequestBodyValidation(t *testing.T) {
	_, p := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var reqBody map[string]interface{}
		if err := json.Unmarshal(body, &reqBody); err != nil {
			t.Fatalf("unmarshal request body: %v", err)
		}

		// Validate system blocks.
		system, ok := reqBody["system"].([]interface{})
		if !ok || len(system) != 2 {
			t.Fatalf("expected 2 system blocks, got %v", reqBody["system"])
		}
		sb0 := system[0].(map[string]interface{})
		if sb0["cache_control"] == nil {
			t.Error("system[0] should have cache_control")
		} else {
			cc := sb0["cache_control"].(map[string]interface{})
			if cc["type"] != "ephemeral" {
				t.Errorf("system[0] cache_control type: expected %q, got %v", "ephemeral", cc["type"])
			}
		}
		sb1 := system[1].(map[string]interface{})
		if sb1["cache_control"] != nil {
			t.Error("system[1] should not have cache_control")
		}

		// Validate thinking.
		thinking, ok := reqBody["thinking"].(map[string]interface{})
		if !ok {
			t.Fatal("expected thinking object in request body")
		}
		if thinking["type"] != "enabled" {
			t.Errorf("thinking type: expected %q, got %v", "enabled", thinking["type"])
		}
		if thinking["budget_tokens"] != float64(10000) {
			t.Errorf("thinking budget_tokens: expected 10000, got %v", thinking["budget_tokens"])
		}

		// Validate stream.
		if reqBody["stream"] != false {
			t.Errorf("stream: expected false, got %v", reqBody["stream"])
		}

		// Validate tools.
		tools, ok := reqBody["tools"].([]interface{})
		if !ok || len(tools) != 1 {
			t.Fatalf("expected 1 tool, got %v", reqBody["tools"])
		}
		tool := tools[0].(map[string]interface{})
		if tool["name"] != "file_read" {
			t.Errorf("tool name: expected %q, got %v", "file_read", tool["name"])
		}

		// Return a valid response.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{
			"id": "msg_test4",
			"type": "message",
			"role": "assistant",
			"content": [{"type": "text", "text": "ok"}],
			"model": "claude-sonnet-4-6-20250514",
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 10, "output_tokens": 2, "cache_read_input_tokens": 0, "cache_creation_input_tokens": 0}
		}`)
	})

	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewUserMessage("hello"),
		},
		SystemBlocks: []provider.SystemBlock{
			{Text: "Base system prompt...", CacheControl: &provider.CacheControl{Type: "ephemeral"}},
			{Text: "Assembled context..."},
		},
		Tools: []provider.ToolDefinition{
			{Name: "file_read", Description: "Read a file", InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)},
		},
		ProviderOptions: NewAnthropicOptions(true, 0),
	}

	_, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
}

// --- Stream Method Integration Tests ---

func TestIntegration_Stream_TextOnly(t *testing.T) {
	_, p := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)

		writeSSE(w, "message_start", `{"type":"message_start","message":{"id":"msg_s1","type":"message","role":"assistant","model":"claude-sonnet-4-6-20250514","usage":{"input_tokens":50,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`)
		writeSSE(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		writeSSE(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`)
		writeSSE(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`)
		writeSSE(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
		writeSSE(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":8}}`)
		writeSSE(w, "message_stop", `{"type":"message_stop"}`)
	})

	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewUserMessage("hi"),
		},
	}

	ch, err := p.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var events []provider.StreamEvent
	for ev := range ch {
		events = append(events, ev)
	}

	// Expected: StreamUsage, TokenDelta("Hello"), TokenDelta(" world"), StreamDone
	if len(events) < 4 {
		t.Fatalf("expected at least 4 events, got %d: %+v", len(events), events)
	}

	// Event 0: StreamUsage
	if su, ok := events[0].(provider.StreamUsage); !ok {
		t.Errorf("event 0: expected StreamUsage, got %T", events[0])
	} else if su.Usage.InputTokens != 50 {
		t.Errorf("event 0: expected InputTokens=50, got %d", su.Usage.InputTokens)
	}

	// Event 1: TokenDelta "Hello"
	if td, ok := events[1].(provider.TokenDelta); !ok {
		t.Errorf("event 1: expected TokenDelta, got %T", events[1])
	} else if td.Text != "Hello" {
		t.Errorf("event 1: expected text %q, got %q", "Hello", td.Text)
	}

	// Event 2: TokenDelta " world"
	if td, ok := events[2].(provider.TokenDelta); !ok {
		t.Errorf("event 2: expected TokenDelta, got %T", events[2])
	} else if td.Text != " world" {
		t.Errorf("event 2: expected text %q, got %q", " world", td.Text)
	}

	// Last event: StreamDone
	lastEvent := events[len(events)-1]
	if sd, ok := lastEvent.(provider.StreamDone); !ok {
		t.Errorf("last event: expected StreamDone, got %T", lastEvent)
	} else {
		if sd.StopReason != provider.StopReasonEndTurn {
			t.Errorf("stop reason: expected %q, got %q", provider.StopReasonEndTurn, sd.StopReason)
		}
		if sd.Usage.OutputTokens != 8 {
			t.Errorf("output tokens: expected 8, got %d", sd.Usage.OutputTokens)
		}
	}
}

func TestIntegration_Stream_ThinkingAndText(t *testing.T) {
	_, p := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)

		writeSSE(w, "message_start", `{"type":"message_start","message":{"id":"msg_s2","type":"message","role":"assistant","model":"claude-sonnet-4-6-20250514","usage":{"input_tokens":80,"output_tokens":0,"cache_read_input_tokens":60,"cache_creation_input_tokens":20}}}`)
		writeSSE(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`)
		writeSSE(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Let me"}}`)
		writeSSE(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":" analyze..."}}`)
		writeSSE(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
		writeSSE(w, "content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`)
		writeSSE(w, "content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"I found the issue."}}`)
		writeSSE(w, "content_block_stop", `{"type":"content_block_stop","index":1}`)
		writeSSE(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":25}}`)
		writeSSE(w, "message_stop", `{"type":"message_stop"}`)
	})

	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewUserMessage("find the bug"),
		},
	}

	ch, err := p.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var events []provider.StreamEvent
	for ev := range ch {
		events = append(events, ev)
	}

	// Expected: StreamUsage, ThinkingDelta("Let me"), ThinkingDelta(" analyze..."), TokenDelta("I found the issue."), StreamDone
	if len(events) < 5 {
		t.Fatalf("expected at least 5 events, got %d: %+v", len(events), events)
	}

	// Event 0: StreamUsage
	if su, ok := events[0].(provider.StreamUsage); !ok {
		t.Errorf("event 0: expected StreamUsage, got %T", events[0])
	} else {
		if su.Usage.InputTokens != 80 {
			t.Errorf("event 0: expected InputTokens=80, got %d", su.Usage.InputTokens)
		}
		if su.Usage.CacheReadTokens != 60 {
			t.Errorf("event 0: expected CacheReadTokens=60, got %d", su.Usage.CacheReadTokens)
		}
		if su.Usage.CacheCreationTokens != 20 {
			t.Errorf("event 0: expected CacheCreationTokens=20, got %d", su.Usage.CacheCreationTokens)
		}
	}

	// Event 1: ThinkingDelta "Let me"
	if td, ok := events[1].(provider.ThinkingDelta); !ok {
		t.Errorf("event 1: expected ThinkingDelta, got %T", events[1])
	} else if td.Thinking != "Let me" {
		t.Errorf("event 1: expected %q, got %q", "Let me", td.Thinking)
	}

	// Event 2: ThinkingDelta " analyze..."
	if td, ok := events[2].(provider.ThinkingDelta); !ok {
		t.Errorf("event 2: expected ThinkingDelta, got %T", events[2])
	} else if td.Thinking != " analyze..." {
		t.Errorf("event 2: expected %q, got %q", " analyze...", td.Thinking)
	}

	// Event 3: TokenDelta "I found the issue."
	if td, ok := events[3].(provider.TokenDelta); !ok {
		t.Errorf("event 3: expected TokenDelta, got %T", events[3])
	} else if td.Text != "I found the issue." {
		t.Errorf("event 3: expected %q, got %q", "I found the issue.", td.Text)
	}

	// Last: StreamDone
	lastEvent := events[len(events)-1]
	if sd, ok := lastEvent.(provider.StreamDone); !ok {
		t.Errorf("last event: expected StreamDone, got %T", lastEvent)
	} else {
		if sd.StopReason != provider.StopReasonEndTurn {
			t.Errorf("stop reason: expected %q, got %q", provider.StopReasonEndTurn, sd.StopReason)
		}
		if sd.Usage.OutputTokens != 25 {
			t.Errorf("output tokens: expected 25, got %d", sd.Usage.OutputTokens)
		}
	}
}

func TestIntegration_Stream_ToolUse(t *testing.T) {
	_, p := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)

		writeSSE(w, "message_start", `{"type":"message_start","message":{"id":"msg_s3","type":"message","role":"assistant","model":"claude-sonnet-4-6-20250514","usage":{"input_tokens":100,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`)
		writeSSE(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		writeSSE(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"I'll read the file."}}`)
		writeSSE(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
		writeSSE(w, "content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_xyz","name":"file_read"}}`)
		writeSSE(w, "content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"path\":"}}`)
		writeSSE(w, "content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"auth.go\"}"}}`)
		writeSSE(w, "content_block_stop", `{"type":"content_block_stop","index":1}`)
		writeSSE(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":35}}`)
		writeSSE(w, "message_stop", `{"type":"message_stop"}`)
	})

	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewUserMessage("read auth.go"),
		},
	}

	ch, err := p.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var events []provider.StreamEvent
	for ev := range ch {
		events = append(events, ev)
	}

	// Expected sequence:
	// 0: StreamUsage
	// 1: TokenDelta "I'll read the file."
	// 2: ToolCallStart{ID: "toolu_xyz", Name: "file_read"}
	// 3: ToolCallDelta{ID: "toolu_xyz", Delta: "{\"path\":"}
	// 4: ToolCallDelta{ID: "toolu_xyz", Delta: "\"auth.go\"}"}
	// 5: ToolCallEnd{ID: "toolu_xyz", Input: {"path":"auth.go"}}
	// 6: StreamDone

	if len(events) < 7 {
		t.Fatalf("expected at least 7 events, got %d: %+v", len(events), events)
	}

	// Event 0: StreamUsage
	if su, ok := events[0].(provider.StreamUsage); !ok {
		t.Errorf("event 0: expected StreamUsage, got %T", events[0])
	} else if su.Usage.InputTokens != 100 {
		t.Errorf("event 0: expected InputTokens=100, got %d", su.Usage.InputTokens)
	}

	// Event 1: TokenDelta
	if td, ok := events[1].(provider.TokenDelta); !ok {
		t.Errorf("event 1: expected TokenDelta, got %T", events[1])
	} else if td.Text != "I'll read the file." {
		t.Errorf("event 1: expected %q, got %q", "I'll read the file.", td.Text)
	}

	// Event 2: ToolCallStart
	if tcs, ok := events[2].(provider.ToolCallStart); !ok {
		t.Errorf("event 2: expected ToolCallStart, got %T", events[2])
	} else {
		if tcs.ID != "toolu_xyz" {
			t.Errorf("event 2 ID: expected %q, got %q", "toolu_xyz", tcs.ID)
		}
		if tcs.Name != "file_read" {
			t.Errorf("event 2 Name: expected %q, got %q", "file_read", tcs.Name)
		}
	}

	// Event 3: ToolCallDelta
	if tcd, ok := events[3].(provider.ToolCallDelta); !ok {
		t.Errorf("event 3: expected ToolCallDelta, got %T", events[3])
	} else {
		if tcd.ID != "toolu_xyz" {
			t.Errorf("event 3 ID: expected %q, got %q", "toolu_xyz", tcd.ID)
		}
		if tcd.Delta != `{"path":` {
			t.Errorf("event 3 Delta: expected %q, got %q", `{"path":`, tcd.Delta)
		}
	}

	// Event 4: ToolCallDelta
	if tcd, ok := events[4].(provider.ToolCallDelta); !ok {
		t.Errorf("event 4: expected ToolCallDelta, got %T", events[4])
	} else {
		if tcd.Delta != `"auth.go"}` {
			t.Errorf("event 4 Delta: expected %q, got %q", `"auth.go"}`, tcd.Delta)
		}
	}

	// Event 5: ToolCallEnd
	if tce, ok := events[5].(provider.ToolCallEnd); !ok {
		t.Errorf("event 5: expected ToolCallEnd, got %T", events[5])
	} else {
		if tce.ID != "toolu_xyz" {
			t.Errorf("event 5 ID: expected %q, got %q", "toolu_xyz", tce.ID)
		}
		expectedInput := `{"path":"auth.go"}`
		if string(tce.Input) != expectedInput {
			t.Errorf("event 5 Input: expected %q, got %q", expectedInput, string(tce.Input))
		}
	}

	// Last: StreamDone
	lastEvent := events[len(events)-1]
	if sd, ok := lastEvent.(provider.StreamDone); !ok {
		t.Errorf("last event: expected StreamDone, got %T", lastEvent)
	} else {
		if sd.StopReason != provider.StopReasonToolUse {
			t.Errorf("stop reason: expected %q, got %q", provider.StopReasonToolUse, sd.StopReason)
		}
		if sd.Usage.OutputTokens != 35 {
			t.Errorf("output tokens: expected 35, got %d", sd.Usage.OutputTokens)
		}
	}
}

// --- Error Handling Integration Tests ---

func TestIntegration_Complete_AuthError(t *testing.T) {
	_, p := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		fmt.Fprint(w, `{"error":{"type":"authentication_error","message":"invalid token"}}`)
	})

	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewUserMessage("hello"),
		},
	}

	_, err := p.Complete(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var pe *provider.ProviderError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *provider.ProviderError, got %T", err)
	}
	if pe.StatusCode != 401 {
		t.Errorf("status code: expected 401, got %d", pe.StatusCode)
	}
	if pe.Retriable {
		t.Error("expected Retriable=false")
	}
	if !containsCI(pe.Message, "authentication failed") {
		t.Errorf("expected message containing %q, got %q", "authentication failed", pe.Message)
	}
}

func TestIntegration_Complete_RateLimitRetry(t *testing.T) {
	var callCount int64
	_, p := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&callCount, 1)
		if n <= 2 {
			w.WriteHeader(429)
			fmt.Fprint(w, `{"error":{"type":"rate_limit_error","message":"rate limit exceeded"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{
			"id": "msg_retry",
			"type": "message",
			"role": "assistant",
			"content": [{"type": "text", "text": "ok"}],
			"model": "claude-sonnet-4-6-20250514",
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 10, "output_tokens": 1, "cache_read_input_tokens": 0, "cache_creation_input_tokens": 0}
		}`)
	})

	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewUserMessage("hello"),
		},
	}

	resp, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if atomic.LoadInt64(&callCount) != 3 {
		t.Errorf("expected 3 requests, got %d", atomic.LoadInt64(&callCount))
	}
}

func TestIntegration_Complete_ServerErrorExhausted(t *testing.T) {
	var callCount int64
	_, p := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&callCount, 1)
		w.WriteHeader(503)
		fmt.Fprint(w, `service unavailable`)
	})

	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewUserMessage("hello"),
		},
	}

	_, err := p.Complete(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var pe *provider.ProviderError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *provider.ProviderError, got %T", err)
	}
	if pe.StatusCode != 503 {
		t.Errorf("status code: expected 503, got %d", pe.StatusCode)
	}
	if !pe.Retriable {
		t.Error("expected Retriable=true")
	}
	if atomic.LoadInt64(&callCount) != 3 {
		t.Errorf("expected 3 requests, got %d", atomic.LoadInt64(&callCount))
	}
}

func TestIntegration_Stream_InitialHTTPError(t *testing.T) {
	var callCount int64
	_, p := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&callCount, 1)
		w.WriteHeader(500)
		fmt.Fprint(w, `internal server error`)
	})

	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewUserMessage("hello"),
		},
	}

	ch, err := p.Stream(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if ch != nil {
		t.Error("expected nil channel")
	}

	var pe *provider.ProviderError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *provider.ProviderError, got %T", err)
	}
	if pe.StatusCode != 500 {
		t.Errorf("status code: expected 500, got %d", pe.StatusCode)
	}
	if !pe.Retriable {
		t.Error("expected Retriable=true")
	}
	if atomic.LoadInt64(&callCount) != 3 {
		t.Errorf("expected 3 requests, got %d", atomic.LoadInt64(&callCount))
	}
}

func TestIntegration_Complete_BadRequest(t *testing.T) {
	var callCount int64
	_, p := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&callCount, 1)
		w.WriteHeader(400)
		fmt.Fprint(w, `{"error":{"type":"invalid_request_error","message":"max_tokens must be positive"}}`)
	})

	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewUserMessage("hello"),
		},
	}

	_, err := p.Complete(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var pe *provider.ProviderError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *provider.ProviderError, got %T", err)
	}
	if pe.StatusCode != 400 {
		t.Errorf("status code: expected 400, got %d", pe.StatusCode)
	}
	if pe.Retriable {
		t.Error("expected Retriable=false")
	}
	if !containsCI(pe.Message, "max_tokens must be positive") {
		t.Errorf("expected message containing error body, got %q", pe.Message)
	}
	if atomic.LoadInt64(&callCount) != 1 {
		t.Errorf("expected 1 request (no retry), got %d", atomic.LoadInt64(&callCount))
	}
}

// --- Context Cancellation Integration Tests ---

func TestIntegration_Complete_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Pre-cancel.

	_, p := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called with cancelled context")
	})

	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewUserMessage("hello"),
		},
	}

	start := time.Now()
	_, err := p.Complete(ctx, req)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("expected prompt return, took %v", elapsed)
	}
}

func TestIntegration_Stream_ContextCancelled(t *testing.T) {
	_, p := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)

		writeSSE(w, "message_start", `{"type":"message_start","message":{"id":"msg_cancel","type":"message","role":"assistant","model":"claude-sonnet-4-6-20250514","usage":{"input_tokens":10,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`)
		writeSSE(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		writeSSE(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`)

		// Simulate slow stream.
		time.Sleep(5 * time.Second)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewUserMessage("hello"),
		},
	}

	ch, err := p.Stream(ctx, req)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	// Read at least one event before cancelling.
	for ev := range ch {
		if _, ok := ev.(provider.TokenDelta); ok {
			cancel()
			break
		}
		// Also accept StreamUsage as first event.
		if _, ok := ev.(provider.StreamUsage); ok {
			continue
		}
	}

	// Drain remaining events (channel should close).
	for range ch {
	}
}

// containsCI performs a case-insensitive contains check.
func containsCI(s, substr string) bool {
	sl := strings.ToLower(s)
	sl2 := strings.ToLower(substr)
	return strings.Contains(sl, sl2)
}
