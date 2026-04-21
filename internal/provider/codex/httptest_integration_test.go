package codex

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

// newHTTPTestProvider creates a CodexProvider with injected state, bypassing
// the real codex CLI check and credential flow. Suitable for httptest-based
// tests that do not require the codex binary.
func newHTTPTestProvider(t *testing.T, serverURL string) *CodexProvider {
	t.Helper()
	return &CodexProvider{
		baseURL:      serverURL,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		cachedToken:  "test-token-abc",
		tokenExpiry:  time.Now().Add(1 * time.Hour),
		codexBinPath: "/usr/bin/true",
	}
}

// ---------- Complete: text response ----------

func TestHTTPTest_CompleteTextResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate method and path.
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/responses" {
			t.Errorf("expected /v1/responses, got %s", r.URL.Path)
		}

		// Validate auth header.
		if got := r.Header.Get("Authorization"); got != "Bearer test-token-abc" {
			t.Errorf("Authorization: expected %q, got %q", "Bearer test-token-abc", got)
		}

		// Validate Content-Type.
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type: expected %q, got %q", "application/json", got)
		}

		// Validate request body.
		body, _ := io.ReadAll(r.Body)
		var reqBody map[string]interface{}
		if err := json.Unmarshal(body, &reqBody); err != nil {
			t.Fatalf("failed to parse request body: %v", err)
		}
		if reqBody["model"] != "gpt-5.4" {
			t.Errorf("expected model %q, got %v", "gpt-5.4", reqBody["model"])
		}
		if reqBody["stream"] != false {
			t.Errorf("expected stream false, got %v", reqBody["stream"])
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id": "resp_ht_001",
			"object": "response",
			"model": "o4-mini",
			"output": [
				{
					"type": "message",
					"id": "msg_1",
					"role": "assistant",
					"content": [{"type": "output_text", "text": "Hello from httptest!"}]
				}
			],
			"usage": {
				"input_tokens": 120,
				"output_tokens": 30,
				"input_tokens_details": {"cached_tokens": 15},
				"output_tokens_details": {"reasoning_tokens": 5}
			}
		}`)
	}))
	defer server.Close()

	p := newHTTPTestProvider(t, server.URL)
	req := &provider.Request{
		Model:    "o4-mini",
		Messages: []provider.Message{provider.NewUserMessage("Hello")},
	}

	resp, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Model != "o4-mini" {
		t.Errorf("expected model %q, got %q", "o4-mini", resp.Model)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(resp.Content))
	}
	if resp.Content[0].Type != "text" {
		t.Errorf("expected type %q, got %q", "text", resp.Content[0].Type)
	}
	if resp.Content[0].Text != "Hello from httptest!" {
		t.Errorf("expected text %q, got %q", "Hello from httptest!", resp.Content[0].Text)
	}
	if resp.StopReason != provider.StopReasonEndTurn {
		t.Errorf("expected stop reason %q, got %q", provider.StopReasonEndTurn, resp.StopReason)
	}
	if resp.Usage.InputTokens != 120 {
		t.Errorf("expected InputTokens 120, got %d", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 30 {
		t.Errorf("expected OutputTokens 30, got %d", resp.Usage.OutputTokens)
	}
	if resp.Usage.CacheReadTokens != 15 {
		t.Errorf("expected CacheReadTokens 15, got %d", resp.Usage.CacheReadTokens)
	}
	if resp.LatencyMs < 0 {
		t.Errorf("expected non-negative LatencyMs, got %d", resp.LatencyMs)
	}
}

// ---------- Complete: tool call / function_call response ----------

func TestHTTPTest_CompleteToolCallResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id": "resp_ht_002",
			"object": "response",
			"model": "o3",
			"output": [
				{
					"type": "reasoning",
					"id": "rs_1",
					"encrypted_content": "encrypted-thinking-data"
				},
				{
					"type": "message",
					"id": "msg_1",
					"role": "assistant",
					"content": [{"type": "output_text", "text": "I will read the file."}]
				},
				{
					"type": "function_call",
					"id": "fc_1",
					"call_id": "call_abc",
					"name": "file_read",
					"arguments": "{\"path\":\"main.go\"}"
				}
			],
			"usage": {
				"input_tokens": 400,
				"output_tokens": 100,
				"input_tokens_details": {"cached_tokens": 50},
				"output_tokens_details": {"reasoning_tokens": 40}
			}
		}`)
	}))
	defer server.Close()

	p := newHTTPTestProvider(t, server.URL)
	req := &provider.Request{
		Model:    "o3",
		Messages: []provider.Message{provider.NewUserMessage("Read main.go")},
		Tools: []provider.ToolDefinition{
			{
				Name:        "file_read",
				Description: "Read a file",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
			},
		},
	}

	resp, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Content) != 3 {
		t.Fatalf("expected 3 content blocks, got %d", len(resp.Content))
	}

	// Block 0: thinking
	if resp.Content[0].Type != "thinking" {
		t.Errorf("block 0: expected type %q, got %q", "thinking", resp.Content[0].Type)
	}
	if resp.Content[0].Thinking != "encrypted-thinking-data" {
		t.Errorf("block 0: expected thinking %q, got %q", "encrypted-thinking-data", resp.Content[0].Thinking)
	}

	// Block 1: text
	if resp.Content[1].Type != "text" {
		t.Errorf("block 1: expected type %q, got %q", "text", resp.Content[1].Type)
	}
	if resp.Content[1].Text != "I will read the file." {
		t.Errorf("block 1: expected text %q, got %q", "I will read the file.", resp.Content[1].Text)
	}

	// Block 2: tool_use
	if resp.Content[2].Type != "tool_use" {
		t.Errorf("block 2: expected type %q, got %q", "tool_use", resp.Content[2].Type)
	}
	if resp.Content[2].ID != "call_abc" {
		t.Errorf("block 2: expected ID %q, got %q", "call_abc", resp.Content[2].ID)
	}
	if resp.Content[2].Name != "file_read" {
		t.Errorf("block 2: expected Name %q, got %q", "file_read", resp.Content[2].Name)
	}
	if string(resp.Content[2].Input) != `{"path":"main.go"}` {
		t.Errorf("block 2: expected Input %q, got %q", `{"path":"main.go"}`, string(resp.Content[2].Input))
	}

	if resp.StopReason != provider.StopReasonToolUse {
		t.Errorf("expected stop reason %q, got %q", provider.StopReasonToolUse, resp.StopReason)
	}
	if resp.Usage.CacheReadTokens != 50 {
		t.Errorf("expected CacheReadTokens 50, got %d", resp.Usage.CacheReadTokens)
	}
}

// ---------- Complete: auth error (401) ----------

func TestHTTPTest_Complete401AuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		fmt.Fprint(w, `{"error":"invalid_token"}`)
	}))
	defer server.Close()

	p := newHTTPTestProvider(t, server.URL)
	req := &provider.Request{
		Model:    "o3",
		Messages: []provider.Message{provider.NewUserMessage("hello")},
	}

	_, err := p.Complete(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for 401 response")
	}

	var pe *provider.ProviderError
	if !errors.As(err, &pe) {
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

// ---------- Complete: 403 forbidden error ----------

func TestHTTPTest_Complete403ForbiddenError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		fmt.Fprint(w, `{"error":"forbidden"}`)
	}))
	defer server.Close()

	p := newHTTPTestProvider(t, server.URL)
	req := &provider.Request{
		Model:    "o3",
		Messages: []provider.Message{provider.NewUserMessage("hello")},
	}

	_, err := p.Complete(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for 403 response")
	}

	var pe *provider.ProviderError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *provider.ProviderError, got %T", err)
	}
	if pe.StatusCode != 403 {
		t.Errorf("expected status 403, got %d", pe.StatusCode)
	}
	if pe.Retriable {
		t.Error("expected Retriable=false for 403 errors")
	}
}

// ---------- Complete: retry on 429 then succeed ----------

func TestHTTPTest_CompleteRetryThenSuccess(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&requestCount, 1)
		if n < 3 {
			w.WriteHeader(429)
			fmt.Fprint(w, `{"error":"rate_limited"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id": "resp_ht_003",
			"object": "response",
			"model": "o3",
			"output": [
				{
					"type": "message",
					"id": "msg_1",
					"role": "assistant",
					"content": [{"type": "output_text", "text": "Success after retries!"}]
				}
			],
			"usage": {
				"input_tokens": 80,
				"output_tokens": 15,
				"input_tokens_details": {"cached_tokens": 0},
				"output_tokens_details": {"reasoning_tokens": 0}
			}
		}`)
	}))
	defer server.Close()

	p := newHTTPTestProvider(t, server.URL)
	req := &provider.Request{
		Model:    "o3",
		Messages: []provider.Message{provider.NewUserMessage("hello")},
	}

	resp, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Content) != 1 || resp.Content[0].Text != "Success after retries!" {
		t.Errorf("expected success text, got %v", resp.Content)
	}
	if got := atomic.LoadInt32(&requestCount); got != 3 {
		t.Errorf("expected 3 requests (2 retries + 1 success), got %d", got)
	}
}

// ---------- Complete: all retries exhausted on 500 ----------

func TestHTTPTest_CompleteRetryExhaustion(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(500)
		fmt.Fprint(w, `internal server error`)
	}))
	defer server.Close()

	p := newHTTPTestProvider(t, server.URL)
	req := &provider.Request{
		Model:    "o3",
		Messages: []provider.Message{provider.NewUserMessage("hello")},
	}

	_, err := p.Complete(context.Background(), req)
	if err == nil {
		t.Fatal("expected error after retry exhaustion")
	}

	var pe *provider.ProviderError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *provider.ProviderError, got %T", err)
	}
	if pe.StatusCode != 500 {
		t.Errorf("expected status 500, got %d", pe.StatusCode)
	}
	if !strings.Contains(pe.Message, "server error after 3 attempts") {
		t.Errorf("expected message containing %q, got %q", "server error after 3 attempts", pe.Message)
	}
	if !pe.Retriable {
		t.Error("expected Retriable=true after retry exhaustion (so router can fallback)")
	}
	if got := atomic.LoadInt32(&requestCount); got != 3 {
		t.Errorf("expected 3 requests, got %d", got)
	}
}

// ---------- Complete: non-retriable 400 error ----------

func TestHTTPTest_Complete400BadRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		fmt.Fprint(w, `{"error":"invalid request body"}`)
	}))
	defer server.Close()

	p := newHTTPTestProvider(t, server.URL)
	req := &provider.Request{
		Model:    "o3",
		Messages: []provider.Message{provider.NewUserMessage("hello")},
	}

	_, err := p.Complete(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for 400 response")
	}

	var pe *provider.ProviderError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *provider.ProviderError, got %T", err)
	}
	if pe.StatusCode != 400 {
		t.Errorf("expected status 400, got %d", pe.StatusCode)
	}
	if pe.Retriable {
		t.Error("expected Retriable=false for 400")
	}
}

// ---------- Complete: request body validation (system + tools) ----------

func TestHTTPTest_CompleteRequestBodyValidation(t *testing.T) {
	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id": "resp_ht_004",
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
				"input_tokens": 50,
				"output_tokens": 5,
				"input_tokens_details": {"cached_tokens": 0},
				"output_tokens_details": {"reasoning_tokens": 0}
			}
		}`)
	}))
	defer server.Close()

	p := newHTTPTestProvider(t, server.URL)
	req := &provider.Request{
		Model: "o3",
		SystemBlocks: []provider.SystemBlock{
			{Text: "You are a coding assistant."},
			{Text: "Use Go."},
		},
		Messages: []provider.Message{
			provider.NewUserMessage("Write tests"),
		},
		Tools: []provider.ToolDefinition{
			{
				Name:        "file_write",
				Description: "Write a file",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}}}`),
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

	if body["model"] != "gpt-5.4" {
		t.Errorf("expected model %q, got %v", "gpt-5.4", body["model"])
	}
	if body["stream"] != false {
		t.Errorf("expected stream false, got %v", body["stream"])
	}
	if body["instructions"] != "You are a coding assistant.\n\nUse Go." {
		t.Errorf("expected instructions %q, got %v", "You are a coding assistant.\n\nUse Go.", body["instructions"])
	}

	input, ok := body["input"].([]interface{})
	if !ok {
		t.Fatalf("expected input array, got %T", body["input"])
	}
	if len(input) != 1 {
		t.Fatalf("expected 1 input item, got %d", len(input))
	}

	// Tools should be present.
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
	if tool["name"] != "file_write" {
		t.Errorf("expected tool name %q, got %v", "file_write", tool["name"])
	}
}

// ---------- Stream: text response ----------

func TestHTTPTest_StreamTextResponse(t *testing.T) {
	ssePayload := strings.Join([]string{
		"event: response.created",
		`data: {"type":"response.created","response":{"id":"resp_ht_005","status":"in_progress"}}`,
		"",
		"event: response.output_item.added",
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant"}}`,
		"",
		"event: response.content_part.added",
		`data: {"type":"response.content_part.added","item_id":"msg_1","content_index":0,"part":{"type":"output_text","text":""}}`,
		"",
		"event: response.output_text.delta",
		`data: {"type":"response.output_text.delta","item_id":"msg_1","content_index":0,"delta":"Streaming "}`,
		"",
		"event: response.output_text.delta",
		`data: {"type":"response.output_text.delta","item_id":"msg_1","content_index":0,"delta":"works!"}`,
		"",
		"event: response.content_part.done",
		`data: {"type":"response.content_part.done","item_id":"msg_1","content_index":0,"part":{"type":"output_text","text":"Streaming works!"}}`,
		"",
		"event: response.output_item.done",
		`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant"}}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed","response":{"id":"resp_ht_005","status":"completed","usage":{"input_tokens":80,"output_tokens":12,"input_tokens_details":{"cached_tokens":10},"output_tokens_details":{"reasoning_tokens":0}}}}`,
		"",
	}, "\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate streaming was requested.
		body, _ := io.ReadAll(r.Body)
		var reqBody map[string]interface{}
		json.Unmarshal(body, &reqBody)
		if reqBody["stream"] != true {
			t.Errorf("expected stream true for Stream(), got %v", reqBody["stream"])
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fmt.Fprint(w, ssePayload)
	}))
	defer server.Close()

	p := newHTTPTestProvider(t, server.URL)
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
	if tokenDeltas[0].Text != "Streaming " {
		t.Errorf("expected first delta %q, got %q", "Streaming ", tokenDeltas[0].Text)
	}
	if tokenDeltas[1].Text != "works!" {
		t.Errorf("expected second delta %q, got %q", "works!", tokenDeltas[1].Text)
	}

	if len(streamDones) != 1 {
		t.Fatalf("expected 1 stream done, got %d", len(streamDones))
	}
	if streamDones[0].StopReason != provider.StopReasonEndTurn {
		t.Errorf("expected stop reason %q, got %q", provider.StopReasonEndTurn, streamDones[0].StopReason)
	}
	if streamDones[0].Usage.InputTokens != 80 {
		t.Errorf("expected InputTokens 80, got %d", streamDones[0].Usage.InputTokens)
	}
	if streamDones[0].Usage.OutputTokens != 12 {
		t.Errorf("expected OutputTokens 12, got %d", streamDones[0].Usage.OutputTokens)
	}
	if streamDones[0].Usage.CacheReadTokens != 10 {
		t.Errorf("expected CacheReadTokens 10, got %d", streamDones[0].Usage.CacheReadTokens)
	}
}

func TestHTTPTest_StreamHandlesLargeSSEToken(t *testing.T) {
	largeDelta := strings.Repeat("A", 70*1024)
	ssePayload := strings.Join([]string{
		"event: response.output_item.added",
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg_large","role":"assistant"}}`,
		"",
		"event: response.output_text.delta",
		fmt.Sprintf(`data: {"type":"response.output_text.delta","item_id":"msg_large","content_index":0,"delta":%q}`, largeDelta),
		"",
		"event: response.completed",
		`data: {"type":"response.completed","response":{"id":"resp_ht_large","status":"completed","usage":{"input_tokens":1,"output_tokens":1,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}}`,
		"",
	}, "\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fmt.Fprint(w, ssePayload)
	}))
	defer server.Close()

	p := newHTTPTestProvider(t, server.URL)
	ch, err := p.Stream(context.Background(), &provider.Request{
		Model:    "o3",
		Messages: []provider.Message{provider.NewUserMessage("hello")},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var gotLarge bool
	for event := range ch {
		switch e := event.(type) {
		case provider.TokenDelta:
			if e.Text == largeDelta {
				gotLarge = true
			}
		case provider.StreamError:
			t.Fatalf("unexpected stream error: %v", e)
		}
	}
	if !gotLarge {
		t.Fatal("expected large token delta to be delivered")
	}
}

func TestHTTPTest_CompleteHandlesLargeSSEToken(t *testing.T) {
	largeDelta := strings.Repeat("B", 70*1024)
	ssePayload := strings.Join([]string{
		"event: response.output_item.added",
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg_large","role":"assistant"}}`,
		"",
		"event: response.output_text.delta",
		fmt.Sprintf(`data: {"type":"response.output_text.delta","item_id":"msg_large","content_index":0,"delta":%q}`, largeDelta),
		"",
		"event: response.completed",
		`data: {"type":"response.completed","response":{"id":"resp_complete_large","status":"completed","usage":{"input_tokens":2,"output_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}}`,
		"",
	}, "\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fmt.Fprint(w, ssePayload)
	}))
	defer server.Close()

	p := newHTTPTestProvider(t, server.URL+"/codex")
	resp, err := p.Complete(context.Background(), &provider.Request{
		Model:    "o3",
		Messages: []provider.Message{provider.NewUserMessage("hello")},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Content) != 1 || resp.Content[0].Text != largeDelta {
		t.Fatalf("expected large streamed text response, got %#v", resp.Content)
	}
}

// ---------- Stream: tool call ----------

func TestHTTPTest_StreamToolCall(t *testing.T) {
	ssePayload := strings.Join([]string{
		"event: response.output_item.added",
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_xyz","name":"file_read"}}`,
		"",
		"event: response.function_call_arguments.delta",
		`data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{\"path\":\""}`,
		"",
		"event: response.function_call_arguments.delta",
		`data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"server.go\"}"}`,
		"",
		"event: response.output_item.done",
		`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_xyz","name":"file_read","arguments":"{\"path\":\"server.go\"}"}}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed","response":{"id":"resp_ht_006","status":"completed","output":[{"type":"function_call","id":"fc_1","call_id":"call_xyz","name":"file_read"}],"usage":{"input_tokens":90,"output_tokens":40,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}}`,
		"",
	}, "\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fmt.Fprint(w, ssePayload)
	}))
	defer server.Close()

	p := newHTTPTestProvider(t, server.URL)
	req := &provider.Request{
		Model:    "o3",
		Messages: []provider.Message{provider.NewUserMessage("read server.go")},
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
	if toolStarts[0].ID != "call_xyz" {
		t.Errorf("expected tool start ID %q, got %q", "call_xyz", toolStarts[0].ID)
	}
	if toolStarts[0].Name != "file_read" {
		t.Errorf("expected tool start Name %q, got %q", "file_read", toolStarts[0].Name)
	}

	if len(toolDeltas) != 2 {
		t.Fatalf("expected 2 tool deltas, got %d", len(toolDeltas))
	}
	if toolDeltas[0].ID != "call_xyz" {
		t.Errorf("expected tool delta ID %q, got %q", "call_xyz", toolDeltas[0].ID)
	}

	if len(toolEnds) != 1 {
		t.Fatalf("expected 1 tool end, got %d", len(toolEnds))
	}
	if toolEnds[0].ID != "call_xyz" {
		t.Errorf("expected tool end ID %q, got %q", "call_xyz", toolEnds[0].ID)
	}
	if string(toolEnds[0].Input) != `{"path":"server.go"}` {
		t.Errorf("expected tool end Input %q, got %q", `{"path":"server.go"}`, string(toolEnds[0].Input))
	}

	if len(streamDones) != 1 {
		t.Fatalf("expected 1 stream done, got %d", len(streamDones))
	}
	if streamDones[0].StopReason != provider.StopReasonToolUse {
		t.Errorf("expected stop reason %q, got %q", provider.StopReasonToolUse, streamDones[0].StopReason)
	}
}

// ---------- Stream: error status codes ----------

func TestHTTPTest_StreamErrorStatuses(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		retriable  bool
		msgContain string
	}{
		{"401 auth error", 401, false, "authentication failed"},
		{"429 rate limit", 429, true, "rate limited"},
		{"500 server error", 500, true, "server error"},
		{"502 bad gateway", 502, true, "server error"},
		{"503 unavailable", 503, true, "server error"},
		{"400 bad request", 400, false, "unexpected status 400"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				fmt.Fprint(w, `{"error":"mock error"}`)
			}))
			defer server.Close()

			p := newHTTPTestProvider(t, server.URL)
			req := &provider.Request{
				Model:    "o3",
				Messages: []provider.Message{provider.NewUserMessage("hello")},
			}

			_, err := p.Stream(context.Background(), req)
			if err == nil {
				t.Fatal("expected error")
			}

			var pe *provider.ProviderError
			if !errors.As(err, &pe) {
				t.Fatalf("expected *provider.ProviderError, got %T", err)
			}
			if pe.StatusCode != tt.statusCode {
				t.Errorf("expected status %d, got %d", tt.statusCode, pe.StatusCode)
			}
			if pe.Retriable != tt.retriable {
				t.Errorf("expected Retriable=%v, got %v", tt.retriable, pe.Retriable)
			}
			if !strings.Contains(strings.ToLower(pe.Message), tt.msgContain) {
				t.Errorf("expected message containing %q, got %q", tt.msgContain, pe.Message)
			}
		})
	}
}

// ---------- Stream: reasoning/thinking deltas ----------

func TestHTTPTest_StreamReasoningDelta(t *testing.T) {
	ssePayload := strings.Join([]string{
		"event: response.output_item.added",
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"reasoning","id":"rs_1"}}`,
		"",
		"event: response.reasoning.delta",
		`data: {"type":"response.reasoning.delta","item_id":"rs_1","delta":"Thinking step 1..."}`,
		"",
		"event: response.reasoning.delta",
		`data: {"type":"response.reasoning.delta","item_id":"rs_1","delta":" step 2."}`,
		"",
		"event: response.output_item.done",
		`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","id":"rs_1"}}`,
		"",
		"event: response.output_item.added",
		`data: {"type":"response.output_item.added","output_index":1,"item":{"type":"message","id":"msg_1","role":"assistant"}}`,
		"",
		"event: response.output_text.delta",
		`data: {"type":"response.output_text.delta","item_id":"msg_1","content_index":0,"delta":"Result."}`,
		"",
		"event: response.output_item.done",
		`data: {"type":"response.output_item.done","output_index":1,"item":{"type":"message","id":"msg_1"}}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed","response":{"id":"resp_ht_007","status":"completed","usage":{"input_tokens":60,"output_tokens":25,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":15}}}}`,
		"",
	}, "\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fmt.Fprint(w, ssePayload)
	}))
	defer server.Close()

	p := newHTTPTestProvider(t, server.URL)
	req := &provider.Request{
		Model:    "o3",
		Messages: []provider.Message{provider.NewUserMessage("think about this")},
	}

	ch, err := p.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var thinkingDeltas []provider.ThinkingDelta
	var tokenDeltas []provider.TokenDelta
	var streamDones []provider.StreamDone

	for event := range ch {
		switch e := event.(type) {
		case provider.ThinkingDelta:
			thinkingDeltas = append(thinkingDeltas, e)
		case provider.TokenDelta:
			tokenDeltas = append(tokenDeltas, e)
		case provider.StreamDone:
			streamDones = append(streamDones, e)
		}
	}

	if len(thinkingDeltas) != 2 {
		t.Fatalf("expected 2 thinking deltas, got %d", len(thinkingDeltas))
	}
	if thinkingDeltas[0].Thinking != "Thinking step 1..." {
		t.Errorf("expected first thinking delta %q, got %q", "Thinking step 1...", thinkingDeltas[0].Thinking)
	}
	if thinkingDeltas[1].Thinking != " step 2." {
		t.Errorf("expected second thinking delta %q, got %q", " step 2.", thinkingDeltas[1].Thinking)
	}

	if len(tokenDeltas) != 1 {
		t.Fatalf("expected 1 token delta, got %d", len(tokenDeltas))
	}
	if tokenDeltas[0].Text != "Result." {
		t.Errorf("expected token delta %q, got %q", "Result.", tokenDeltas[0].Text)
	}

	if len(streamDones) != 1 {
		t.Fatalf("expected 1 stream done, got %d", len(streamDones))
	}
	if streamDones[0].StopReason != provider.StopReasonEndTurn {
		t.Errorf("expected stop reason %q, got %q", provider.StopReasonEndTurn, streamDones[0].StopReason)
	}
}

// ---------- Complete: context cancellation ----------

func TestHTTPTest_CompleteContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	p := newHTTPTestProvider(t, "http://localhost:0")
	req := &provider.Request{
		Model:    "o3",
		Messages: []provider.Message{provider.NewUserMessage("hello")},
	}

	_, err := p.Complete(ctx, req)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}

	var pe *provider.ProviderError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *provider.ProviderError, got %T", err)
	}
	if pe.Retriable {
		t.Error("expected Retriable=false for cancelled context")
	}
}

// ---------- Stream: context cancellation mid-stream ----------

func TestHTTPTest_StreamContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected http.Flusher")
		}

		// Send one delta event.
		fmt.Fprint(w, "event: response.output_text.delta\n")
		fmt.Fprint(w, `data: {"type":"response.output_text.delta","item_id":"msg_1","content_index":0,"delta":"partial "}`+"\n\n")
		flusher.Flush()

		// Block until the client disconnects.
		<-r.Context().Done()
	}))
	defer server.Close()

	p := newHTTPTestProvider(t, server.URL)
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
		switch e := event.(type) {
		case provider.TokenDelta:
			gotDelta = true
			if e.Text != "partial " {
				t.Errorf("expected delta %q, got %q", "partial ", e.Text)
			}
		case provider.StreamError:
			if e.Fatal {
				gotFatalError = true
			}
		}
	}

	if !gotDelta {
		t.Error("expected at least one token delta before cancellation")
	}
	if !gotFatalError {
		t.Error("expected a fatal stream error from context cancellation")
	}
}
