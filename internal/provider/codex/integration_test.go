//go:build integration

package codex

import (
	"context"
	"encoding/json"
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

// newTestProvider creates a CodexProvider with injected state, bypassing
// the real credential flow.
func newTestProvider(t *testing.T, serverURL string) *CodexProvider {
	t.Helper()
	return &CodexProvider{
		baseURL:      serverURL,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		cachedToken:  "test-token-123",
		tokenExpiry:  time.Now().Add(1 * time.Hour),
		codexBinPath: "/usr/bin/true",
	}
}

func TestIntegration_CompleteTextResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate auth header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token-123" {
			t.Errorf("expected Authorization %q, got %q", "Bearer test-token-123", auth)
		}

		// Validate request body
		body, _ := io.ReadAll(r.Body)
		var reqBody map[string]interface{}
		if err := json.Unmarshal(body, &reqBody); err != nil {
			t.Fatalf("failed to parse request body: %v", err)
		}
		if reqBody["model"] != "gpt-5.5" {
			t.Errorf("expected model %q, got %v", "gpt-5.5", reqBody["model"])
		}
		if reqBody["stream"] != false {
			t.Errorf("expected stream false, got %v", reqBody["stream"])
		}
		if _, ok := reqBody["input"]; !ok {
			t.Error("expected input array in request")
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id": "resp_test_001",
			"object": "response",
			"model": "o3",
			"output": [
				{
					"type": "message",
					"id": "msg_1",
					"role": "assistant",
					"content": [{"type": "output_text", "text": "The bug is in the auth handler."}]
				}
			],
			"usage": {
				"input_tokens": 200,
				"output_tokens": 50,
				"input_tokens_details": {"cached_tokens": 0},
				"output_tokens_details": {"reasoning_tokens": 0}
			}
		}`)
	}))
	defer server.Close()

	p := newTestProvider(t, server.URL)
	req := &provider.Request{
		Model: "o3",
		Messages: []provider.Message{
			provider.NewUserMessage("Fix the auth bug"),
		},
	}

	resp, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(resp.Content))
	}
	if resp.Content[0].Type != "text" {
		t.Errorf("expected type %q, got %q", "text", resp.Content[0].Type)
	}
	if resp.Content[0].Text != "The bug is in the auth handler." {
		t.Errorf("expected text %q, got %q", "The bug is in the auth handler.", resp.Content[0].Text)
	}
	if resp.StopReason != provider.StopReasonEndTurn {
		t.Errorf("expected stop reason %q, got %q", provider.StopReasonEndTurn, resp.StopReason)
	}
	if resp.Usage.InputTokens != 200 {
		t.Errorf("expected InputTokens 200, got %d", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 50 {
		t.Errorf("expected OutputTokens 50, got %d", resp.Usage.OutputTokens)
	}
}

func TestIntegration_CompleteToolCallResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id": "resp_test_002",
			"object": "response",
			"model": "o3",
			"output": [
				{
					"type": "reasoning",
					"id": "rs_1",
					"encrypted_content": "base64encrypteddata"
				},
				{
					"type": "message",
					"id": "msg_1",
					"role": "assistant",
					"content": [{"type": "output_text", "text": "Let me read that file."}]
				},
				{
					"type": "function_call",
					"id": "fc_1",
					"call_id": "call_1",
					"name": "file_read",
					"arguments": "{\"path\":\"auth.go\"}"
				}
			],
			"usage": {
				"input_tokens": 500,
				"output_tokens": 150,
				"input_tokens_details": {"cached_tokens": 100},
				"output_tokens_details": {"reasoning_tokens": 80}
			}
		}`)
	}))
	defer server.Close()

	p := newTestProvider(t, server.URL)
	req := &provider.Request{
		Model: "o3",
		Messages: []provider.Message{
			provider.NewUserMessage("Read auth.go"),
		},
	}

	resp, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Content) != 3 {
		t.Fatalf("expected 3 content blocks, got %d", len(resp.Content))
	}

	// Block 0: encrypted Codex reasoning
	if resp.Content[0].Type != "codex_reasoning" {
		t.Errorf("block 0: expected type %q, got %q", "codex_reasoning", resp.Content[0].Type)
	}
	if resp.Content[0].EncryptedContent != "base64encrypteddata" || resp.Content[0].ReasoningID != "rs_1" {
		t.Errorf("block 0: expected encrypted reasoning id/data, got %+v", resp.Content[0])
	}

	// Block 1: text
	if resp.Content[1].Type != "text" {
		t.Errorf("block 1: expected type %q, got %q", "text", resp.Content[1].Type)
	}
	if resp.Content[1].Text != "Let me read that file." {
		t.Errorf("block 1: expected text %q, got %q", "Let me read that file.", resp.Content[1].Text)
	}

	// Block 2: tool_use
	if resp.Content[2].Type != "tool_use" {
		t.Errorf("block 2: expected type %q, got %q", "tool_use", resp.Content[2].Type)
	}
	if resp.Content[2].ID != "call_1" {
		t.Errorf("block 2: expected ID %q, got %q", "call_1", resp.Content[2].ID)
	}
	if resp.Content[2].Name != "file_read" {
		t.Errorf("block 2: expected Name %q, got %q", "file_read", resp.Content[2].Name)
	}
	if string(resp.Content[2].Input) != `{"path":"auth.go"}` {
		t.Errorf("block 2: expected Input %q, got %q", `{"path":"auth.go"}`, string(resp.Content[2].Input))
	}

	if resp.StopReason != provider.StopReasonToolUse {
		t.Errorf("expected stop reason %q, got %q", provider.StopReasonToolUse, resp.StopReason)
	}
	if resp.Usage.CacheReadTokens != 100 {
		t.Errorf("expected CacheReadTokens 100, got %d", resp.Usage.CacheReadTokens)
	}
	if resp.Usage.CacheCreationTokens != 0 {
		t.Errorf("expected CacheCreationTokens 0, got %d", resp.Usage.CacheCreationTokens)
	}
}

func TestIntegration_Complete401Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		fmt.Fprint(w, `{"error": "invalid_token"}`)
	}))
	defer server.Close()

	p := newTestProvider(t, server.URL)
	req := &provider.Request{
		Model:    "o3",
		Messages: []provider.Message{provider.NewUserMessage("hello")},
	}

	_, err := p.Complete(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}

	pe, ok := err.(*provider.ProviderError)
	if !ok {
		t.Fatalf("expected *provider.ProviderError, got %T", err)
	}
	if pe.StatusCode != 401 {
		t.Errorf("expected status 401, got %d", pe.StatusCode)
	}
	if !strings.Contains(pe.Message, "Codex authentication failed") {
		t.Errorf("expected message containing %q, got %q", "Codex authentication failed", pe.Message)
	}
	if pe.Retriable {
		t.Error("expected Retriable=false for auth errors")
	}
}

func TestIntegration_Complete429RateLimit(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&requestCount, 1)
		if n < 3 {
			w.WriteHeader(429)
			fmt.Fprint(w, `{"error": "rate_limited"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id": "resp_test_003",
			"object": "response",
			"model": "o3",
			"output": [
				{
					"type": "message",
					"id": "msg_1",
					"role": "assistant",
					"content": [{"type": "output_text", "text": "Success after retries."}]
				}
			],
			"usage": {
				"input_tokens": 100,
				"output_tokens": 10,
				"input_tokens_details": {"cached_tokens": 0},
				"output_tokens_details": {"reasoning_tokens": 0}
			}
		}`)
	}))
	defer server.Close()

	p := newTestProvider(t, server.URL)
	req := &provider.Request{
		Model:    "o3",
		Messages: []provider.Message{provider.NewUserMessage("hello")},
	}

	resp, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Content) != 1 || resp.Content[0].Text != "Success after retries." {
		t.Errorf("expected successful response text, got %v", resp.Content)
	}

	if atomic.LoadInt32(&requestCount) != 3 {
		t.Errorf("expected 3 requests, got %d", atomic.LoadInt32(&requestCount))
	}
}

func TestIntegration_Complete500RetryExhaustion(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(500)
		fmt.Fprint(w, "internal server error")
	}))
	defer server.Close()

	p := newTestProvider(t, server.URL)
	req := &provider.Request{
		Model:    "o3",
		Messages: []provider.Message{provider.NewUserMessage("hello")},
	}

	_, err := p.Complete(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}

	pe, ok := err.(*provider.ProviderError)
	if !ok {
		t.Fatalf("expected *provider.ProviderError, got %T", err)
	}
	if pe.StatusCode != 500 {
		t.Errorf("expected status 500, got %d", pe.StatusCode)
	}
	if !strings.Contains(pe.Message, "server error after 3 attempts") {
		t.Errorf("expected message containing %q, got %q", "server error after 3 attempts", pe.Message)
	}
	if !pe.Retriable {
		t.Error("expected Retriable=true after retry exhaustion so router can fallback")
	}

	if atomic.LoadInt32(&requestCount) != 3 {
		t.Errorf("expected 3 requests, got %d", atomic.LoadInt32(&requestCount))
	}
}

func TestIntegration_CompleteRequestBodyValidation(t *testing.T) {
	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id": "resp_test_004",
			"object": "response",
			"model": "o3",
			"output": [
				{
					"type": "message",
					"id": "msg_1",
					"role": "assistant",
					"content": [{"type": "output_text", "text": "OK"}]
				}
			],
			"usage": {
				"input_tokens": 100,
				"output_tokens": 10,
				"input_tokens_details": {"cached_tokens": 0},
				"output_tokens_details": {"reasoning_tokens": 0}
			}
		}`)
	}))
	defer server.Close()

	p := newTestProvider(t, server.URL)
	req := &provider.Request{
		Model: "o3",
		SystemBlocks: []provider.SystemBlock{
			{Text: "You are helpful."},
			{Text: "Context: Go project"},
		},
		Messages: []provider.Message{
			provider.NewUserMessage("Fix the bug"),
		},
		Tools: []provider.ToolDefinition{
			{
				Name:        "file_read",
				Description: "Read a file",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
			},
		},
	}

	_, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(capturedBody, &body); err != nil {
		t.Fatalf("failed to parse captured body: %v", err)
	}

	if body["model"] != "gpt-5.5" {
		t.Errorf("expected model %q, got %v", "gpt-5.5", body["model"])
	}
	if body["stream"] != false {
		t.Errorf("expected stream false, got %v", body["stream"])
	}
	if body["instructions"] != "You are helpful.\n\nContext: Go project" {
		t.Errorf("expected instructions %q, got %v", "You are helpful.\n\nContext: Go project", body["instructions"])
	}

	input, ok := body["input"].([]interface{})
	if !ok {
		t.Fatalf("expected input array, got %T", body["input"])
	}
	if len(input) != 1 {
		t.Fatalf("expected 1 input item, got %d", len(input))
	}

	usr, ok := input[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected map for user item, got %T", input[0])
	}
	if usr["role"] != "user" {
		t.Errorf("expected role %q, got %v", "user", usr["role"])
	}
	if usr["content"] != "Fix the bug" {
		t.Errorf("expected user content %q, got %v", "Fix the bug", usr["content"])
	}

	// Tools
	tools, ok := body["tools"].([]interface{})
	if !ok {
		t.Fatalf("expected tools array, got %T", body["tools"])
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	tool, ok := tools[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected map for tool, got %T", tools[0])
	}
	if tool["type"] != "function" {
		t.Errorf("expected tool type %q, got %v", "function", tool["type"])
	}
	if tool["name"] != "file_read" {
		t.Errorf("expected tool name %q, got %v", "file_read", tool["name"])
	}

	// Reasoning
	reasoning, ok := body["reasoning"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected reasoning map, got %T", body["reasoning"])
	}
	if reasoning["effort"] != "medium" {
		t.Errorf("expected effort %q, got %v", "medium", reasoning["effort"])
	}
	if reasoning["summary"] != "auto" {
		t.Errorf("expected summary %q, got %v", "auto", reasoning["summary"])
	}
	include, ok := body["include"].([]interface{})
	if !ok {
		t.Fatalf("expected include array, got %T", body["include"])
	}
	if len(include) != 1 || include[0] != "reasoning.encrypted_content" {
		t.Errorf("expected encrypted reasoning include, got %#v", include)
	}
}

func TestIntegration_StreamTextResponse(t *testing.T) {
	ssePayload := strings.Join([]string{
		"event: response.created",
		`data: {"type":"response.created","response":{"id":"resp_test_003","status":"in_progress"}}`,
		"",
		"event: response.output_item.added",
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant"}}`,
		"",
		"event: response.content_part.added",
		`data: {"type":"response.content_part.added","item_id":"msg_1","content_index":0,"part":{"type":"output_text","text":""}}`,
		"",
		"event: response.output_text.delta",
		`data: {"type":"response.output_text.delta","item_id":"msg_1","content_index":0,"delta":"Hello "}`,
		"",
		"event: response.output_text.delta",
		`data: {"type":"response.output_text.delta","item_id":"msg_1","content_index":0,"delta":"world!"}`,
		"",
		"event: response.content_part.done",
		`data: {"type":"response.content_part.done","item_id":"msg_1","content_index":0,"part":{"type":"output_text","text":"Hello world!"}}`,
		"",
		"event: response.output_item.done",
		`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"Hello world!"}]}}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed","response":{"id":"resp_test_003","status":"completed","usage":{"input_tokens":100,"output_tokens":10,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}}`,
		"",
	}, "\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fmt.Fprint(w, ssePayload)
	}))
	defer server.Close()

	p := newTestProvider(t, server.URL)
	req := &provider.Request{
		Model:    "o3",
		Messages: []provider.Message{provider.NewUserMessage("hello")},
	}

	ch, err := p.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var tokenDeltas []provider.TokenDelta
	var streamDones []provider.StreamDone
	var streamErrors []provider.StreamError

	for event := range ch {
		switch e := event.(type) {
		case provider.TokenDelta:
			tokenDeltas = append(tokenDeltas, e)
		case provider.StreamDone:
			streamDones = append(streamDones, e)
		case provider.StreamError:
			streamErrors = append(streamErrors, e)
		}
	}

	if len(streamErrors) > 0 {
		t.Errorf("unexpected stream errors: %v", streamErrors)
	}

	if len(tokenDeltas) != 2 {
		t.Fatalf("expected 2 token deltas, got %d", len(tokenDeltas))
	}
	if tokenDeltas[0].Text != "Hello " {
		t.Errorf("expected first delta %q, got %q", "Hello ", tokenDeltas[0].Text)
	}
	if tokenDeltas[1].Text != "world!" {
		t.Errorf("expected second delta %q, got %q", "world!", tokenDeltas[1].Text)
	}

	if len(streamDones) != 1 {
		t.Fatalf("expected 1 stream done, got %d", len(streamDones))
	}
	if streamDones[0].StopReason != provider.StopReasonEndTurn {
		t.Errorf("expected stop reason %q, got %q", provider.StopReasonEndTurn, streamDones[0].StopReason)
	}
	if streamDones[0].Usage.InputTokens != 100 {
		t.Errorf("expected InputTokens 100, got %d", streamDones[0].Usage.InputTokens)
	}
	if streamDones[0].Usage.OutputTokens != 10 {
		t.Errorf("expected OutputTokens 10, got %d", streamDones[0].Usage.OutputTokens)
	}
}

func TestIntegration_StreamToolCall(t *testing.T) {
	ssePayload := strings.Join([]string{
		"event: response.output_item.added",
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"file_read"}}`,
		"",
		"event: response.function_call_arguments.delta",
		`data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{\"path\":"}`,
		"",
		"event: response.function_call_arguments.delta",
		`data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"\"auth.go\"}"}`,
		"",
		"event: response.output_item.done",
		`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"file_read","arguments":"{\"path\":\"auth.go\"}"}}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed","response":{"id":"resp_test_004","status":"completed","output":[{"type":"function_call","id":"fc_1","call_id":"call_1","name":"file_read"}],"usage":{"input_tokens":100,"output_tokens":50,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}}`,
		"",
	}, "\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fmt.Fprint(w, ssePayload)
	}))
	defer server.Close()

	p := newTestProvider(t, server.URL)
	req := &provider.Request{
		Model:    "o3",
		Messages: []provider.Message{provider.NewUserMessage("read auth.go")},
	}

	ch, err := p.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var toolStarts []provider.ToolCallStart
	var toolDeltas []provider.ToolCallDelta
	var toolEnds []provider.ToolCallEnd
	var streamDones []provider.StreamDone

	for event := range ch {
		switch e := event.(type) {
		case provider.ToolCallStart:
			toolStarts = append(toolStarts, e)
		case provider.ToolCallDelta:
			toolDeltas = append(toolDeltas, e)
		case provider.ToolCallEnd:
			toolEnds = append(toolEnds, e)
		case provider.StreamDone:
			streamDones = append(streamDones, e)
		}
	}

	if len(toolStarts) != 1 {
		t.Fatalf("expected 1 tool start, got %d", len(toolStarts))
	}
	if toolStarts[0].ID != "call_1" {
		t.Errorf("expected tool start ID %q, got %q", "call_1", toolStarts[0].ID)
	}
	if toolStarts[0].Name != "file_read" {
		t.Errorf("expected tool start Name %q, got %q", "file_read", toolStarts[0].Name)
	}

	if len(toolDeltas) != 2 {
		t.Fatalf("expected 2 tool deltas, got %d", len(toolDeltas))
	}
	if toolDeltas[0].ID != "call_1" {
		t.Errorf("expected tool delta ID %q, got %q", "call_1", toolDeltas[0].ID)
	}

	if len(toolEnds) != 1 {
		t.Fatalf("expected 1 tool end, got %d", len(toolEnds))
	}
	if toolEnds[0].ID != "call_1" {
		t.Errorf("expected tool end ID %q, got %q", "call_1", toolEnds[0].ID)
	}
	if string(toolEnds[0].Input) != `{"path":"auth.go"}` {
		t.Errorf("expected tool end Input %q, got %q", `{"path":"auth.go"}`, string(toolEnds[0].Input))
	}

	if len(streamDones) != 1 {
		t.Fatalf("expected 1 stream done, got %d", len(streamDones))
	}
	if streamDones[0].StopReason != provider.StopReasonToolUse {
		t.Errorf("expected stop reason %q, got %q", provider.StopReasonToolUse, streamDones[0].StopReason)
	}
}

func TestIntegration_StreamContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected http.Flusher")
		}

		// Send one delta event
		fmt.Fprint(w, "event: response.output_text.delta\n")
		fmt.Fprint(w, `data: {"type":"response.output_text.delta","item_id":"msg_1","content_index":0,"delta":"Hello "}`+"\n\n")
		flusher.Flush()

		// Block until the request context is done (client disconnects)
		<-r.Context().Done()
	}))
	defer server.Close()

	p := newTestProvider(t, server.URL)
	req := &provider.Request{
		Model:    "o3",
		Messages: []provider.Message{provider.NewUserMessage("hello")},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	ch, err := p.Stream(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var gotDelta bool
	var gotFatalError bool

	for event := range ch {
		switch event.(type) {
		case provider.TokenDelta:
			gotDelta = true
		case provider.StreamError:
			se := event.(provider.StreamError)
			if se.Fatal {
				gotFatalError = true
			}
		}
	}

	if !gotDelta {
		t.Error("expected at least one token delta")
	}
	if !gotFatalError {
		t.Error("expected a fatal stream error from context cancellation")
	}
}
