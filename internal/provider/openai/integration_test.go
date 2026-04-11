package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/provider"
)

func newTestProvider(t *testing.T, serverURL string, apiKey string) *OpenAIProvider {
	t.Helper()
	p, err := NewOpenAIProvider(OpenAIConfig{
		Name:          "test",
		BaseURL:       serverURL,
		APIKey:        apiKey,
		Model:         "test-model",
		ContextLength: 4096,
	})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	return p
}

func TestIntegration_SuccessfulCompletion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Helper()

		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Authorization") != "Bearer test-key-123" {
			t.Errorf("expected Authorization 'Bearer test-key-123', got %q", r.Header.Get("Authorization"))
		}

		var body chatRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		if body.Model != "test-model" {
			t.Errorf("expected model 'test-model', got %q", body.Model)
		}
		if body.Stream {
			t.Errorf("expected stream=false")
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id": "chatcmpl-test1",
			"object": "chat.completion",
			"model": "test-model",
			"choices": [
				{
					"index": 0,
					"message": {"role": "assistant", "content": "The fix is to add nil checks."},
					"finish_reason": "stop"
				}
			],
			"usage": {"prompt_tokens": 42, "completion_tokens": 15, "total_tokens": 57}
		}`)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL, "test-key-123")
	ctx := context.Background()
	req := &provider.Request{
		Messages: []provider.Message{
			provider.NewUserMessage("Fix the auth bug"),
		},
	}

	resp, err := p.Complete(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(resp.Content))
	}
	if resp.Content[0].Type != "text" {
		t.Errorf("expected type 'text', got %q", resp.Content[0].Type)
	}
	if resp.Content[0].Text != "The fix is to add nil checks." {
		t.Errorf("expected text %q, got %q", "The fix is to add nil checks.", resp.Content[0].Text)
	}
	if resp.StopReason != provider.StopReasonEndTurn {
		t.Errorf("expected StopReasonEndTurn, got %q", resp.StopReason)
	}
	if resp.Usage.InputTokens != 42 {
		t.Errorf("expected InputTokens 42, got %d", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 15 {
		t.Errorf("expected OutputTokens 15, got %d", resp.Usage.OutputTokens)
	}
	if resp.Usage.CacheReadTokens != 0 {
		t.Errorf("expected CacheReadTokens 0, got %d", resp.Usage.CacheReadTokens)
	}
	if resp.Usage.CacheCreationTokens != 0 {
		t.Errorf("expected CacheCreationTokens 0, got %d", resp.Usage.CacheCreationTokens)
	}
}

func TestIntegration_CompletionWithToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id": "chatcmpl-test2",
			"object": "chat.completion",
			"model": "test-model",
			"choices": [
				{
					"index": 0,
					"message": {
						"role": "assistant",
						"content": "Let me read the file.",
						"tool_calls": [
							{
								"id": "call_abc",
								"type": "function",
								"function": {"name": "file_read", "arguments": "{\"path\":\"main.go\"}"}
							}
						]
					},
					"finish_reason": "tool_calls"
				}
			],
			"usage": {"prompt_tokens": 80, "completion_tokens": 25, "total_tokens": 105}
		}`)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL, "test-key")
	resp, err := p.Complete(context.Background(), &provider.Request{
		Messages: []provider.Message{provider.NewUserMessage("read main.go")},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(resp.Content))
	}
	if resp.Content[0].Type != "text" {
		t.Errorf("expected text block first, got %q", resp.Content[0].Type)
	}
	if resp.Content[0].Text != "Let me read the file." {
		t.Errorf("expected text %q, got %q", "Let me read the file.", resp.Content[0].Text)
	}

	toolBlock := resp.Content[1]
	if toolBlock.Type != "tool_use" {
		t.Fatalf("expected tool_use block, got %q", toolBlock.Type)
	}
	if toolBlock.ID != "call_abc" {
		t.Errorf("expected ID 'call_abc', got %q", toolBlock.ID)
	}
	if toolBlock.Name != "file_read" {
		t.Errorf("expected name 'file_read', got %q", toolBlock.Name)
	}
	if string(toolBlock.Input) != `{"path":"main.go"}` {
		t.Errorf("expected input %q, got %q", `{"path":"main.go"}`, string(toolBlock.Input))
	}
	if resp.StopReason != provider.StopReasonToolUse {
		t.Errorf("expected StopReasonToolUse, got %q", resp.StopReason)
	}
}

func TestIntegration_NoAuthorizationHeaderWhenKeyEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "" {
			t.Errorf("expected no Authorization header, got %q", auth)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id": "chatcmpl-noauth",
			"object": "chat.completion",
			"model": "test-model",
			"choices": [{"index": 0, "message": {"role": "assistant", "content": "ok"}, "finish_reason": "stop"}],
			"usage": {"prompt_tokens": 10, "completion_tokens": 1, "total_tokens": 11}
		}`)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL, "") // empty API key
	_, err := p.Complete(context.Background(), &provider.Request{
		Messages: []provider.Message{provider.NewUserMessage("hello")},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIntegration_AuthenticationError401(t *testing.T) {
	var requestCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(401)
		fmt.Fprint(w, `{"error":{"message":"Invalid API key"}}`)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL, "bad-key")
	_, err := p.Complete(context.Background(), &provider.Request{
		Messages: []provider.Message{provider.NewUserMessage("hello")},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "authentication failed. Check API key configuration.") {
		t.Errorf("expected auth error message, got %q", err.Error())
	}
	if atomic.LoadInt32(&requestCount) != 1 {
		t.Errorf("expected 1 request (no retries), got %d", atomic.LoadInt32(&requestCount))
	}
}

func TestIntegration_RateLimitWithRetry(t *testing.T) {
	var requestCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)
		if count < 3 {
			w.WriteHeader(429)
			fmt.Fprint(w, `{"error":{"message":"Rate limit exceeded"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id": "chatcmpl-retry",
			"object": "chat.completion",
			"model": "test-model",
			"choices": [{"index": 0, "message": {"role": "assistant", "content": "success after retry"}, "finish_reason": "stop"}],
			"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
		}`)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL, "test-key")
	start := time.Now()
	resp, err := p.Complete(context.Background(), &provider.Request{
		Messages: []provider.Message{provider.NewUserMessage("hello")},
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content[0].Text != "success after retry" {
		t.Errorf("expected 'success after retry', got %q", resp.Content[0].Text)
	}
	if atomic.LoadInt32(&requestCount) != 3 {
		t.Errorf("expected 3 requests, got %d", atomic.LoadInt32(&requestCount))
	}
	// Verify backoff occurred: at least 1s (first backoff) + some overhead.
	if elapsed < 1*time.Second {
		t.Errorf("expected at least 1s elapsed for backoff, got %v", elapsed)
	}
}

func TestIntegration_ServerErrorExhaustsRetries(t *testing.T) {
	var requestCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(500)
		fmt.Fprint(w, `{"error":{"message":"Internal server error"}}`)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL, "test-key")
	_, err := p.Complete(context.Background(), &provider.Request{
		Messages: []provider.Message{provider.NewUserMessage("hello")},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "server error (HTTP 500) after 3 attempts") {
		t.Errorf("expected server error message, got %q", err.Error())
	}
	if atomic.LoadInt32(&requestCount) != 3 {
		t.Errorf("expected 3 requests, got %d", atomic.LoadInt32(&requestCount))
	}
}

func TestIntegration_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL, "test-key")
	_, err := p.Complete(ctx, &provider.Request{
		Messages: []provider.Message{provider.NewUserMessage("hello")},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	// The error should be related to context cancellation.
	if !strings.Contains(err.Error(), "canceled") && !strings.Contains(err.Error(), "cancelled") {
		t.Errorf("expected context cancellation error, got %q", err.Error())
	}
}

func TestIntegration_SuccessfulSSEStreaming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)

		lines := []string{
			`data: {"id":"chatcmpl-stream1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
			"",
			`data: {"id":"chatcmpl-stream1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
			"",
			`data: {"id":"chatcmpl-stream1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}`,
			"",
			`data: {"id":"chatcmpl-stream1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			"",
			"data: [DONE]",
		}

		flusher, ok := w.(http.Flusher)
		for _, line := range lines {
			fmt.Fprintln(w, line)
			if ok {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL, "test-key")
	ch, err := p.Stream(context.Background(), &provider.Request{
		Messages: []provider.Message{provider.NewUserMessage("hello")},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var events []provider.StreamEvent
	timeout := time.After(5 * time.Second)
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				goto done
			}
			events = append(events, event)
		case <-timeout:
			t.Fatal("timed out waiting for stream events")
		}
	}
done:

	// Expect: TokenDelta("Hello"), TokenDelta(" world"), StreamDone(EndTurn)
	var textEvents []provider.TokenDelta
	var doneEvents []provider.StreamDone
	for _, e := range events {
		switch v := e.(type) {
		case provider.TokenDelta:
			textEvents = append(textEvents, v)
		case provider.StreamDone:
			doneEvents = append(doneEvents, v)
		}
	}

	if len(textEvents) != 2 {
		t.Fatalf("expected 2 text events, got %d", len(textEvents))
	}
	if textEvents[0].Text != "Hello" {
		t.Errorf("expected first text 'Hello', got %q", textEvents[0].Text)
	}
	if textEvents[1].Text != " world" {
		t.Errorf("expected second text ' world', got %q", textEvents[1].Text)
	}

	if len(doneEvents) != 1 {
		t.Fatalf("expected 1 done event, got %d", len(doneEvents))
	}
	if doneEvents[0].StopReason != provider.StopReasonEndTurn {
		t.Errorf("expected StopReasonEndTurn, got %q", doneEvents[0].StopReason)
	}
}

func TestIntegration_SSEStreamingWithToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)

		lines := []string{
			`data: {"id":"chatcmpl-stream2","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
			"",
			`data: {"id":"chatcmpl-stream2","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"Let me check."},"finish_reason":null}]}`,
			"",
			`data: {"id":"chatcmpl-stream2","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_s1","type":"function","function":{"name":"file_read","arguments":""}}]},"finish_reason":null}]}`,
			"",
			`data: {"id":"chatcmpl-stream2","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":"}}]},"finish_reason":null}]}`,
			"",
			`data: {"id":"chatcmpl-stream2","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"auth.go\"}"}}]},"finish_reason":null}]}`,
			"",
			`data: {"id":"chatcmpl-stream2","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
			"",
			"data: [DONE]",
		}

		flusher, ok := w.(http.Flusher)
		for _, line := range lines {
			fmt.Fprintln(w, line)
			if ok {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL, "test-key")
	ch, err := p.Stream(context.Background(), &provider.Request{
		Messages: []provider.Message{provider.NewUserMessage("check the code")},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var events []provider.StreamEvent
	timeout := time.After(5 * time.Second)
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				goto done
			}
			events = append(events, event)
		case <-timeout:
			t.Fatal("timed out waiting for stream events")
		}
	}
done:

	// Expect: TokenDelta("Let me check."), ToolCallStart, ToolCallEnd, StreamDone(ToolUse)
	var textEvents []provider.TokenDelta
	var toolStarts []provider.ToolCallStart
	var toolEnds []provider.ToolCallEnd
	var doneEvents []provider.StreamDone
	for _, e := range events {
		switch v := e.(type) {
		case provider.TokenDelta:
			textEvents = append(textEvents, v)
		case provider.ToolCallStart:
			toolStarts = append(toolStarts, v)
		case provider.ToolCallEnd:
			toolEnds = append(toolEnds, v)
		case provider.StreamDone:
			doneEvents = append(doneEvents, v)
		}
	}

	if len(textEvents) != 1 {
		t.Fatalf("expected 1 text event, got %d", len(textEvents))
	}
	if textEvents[0].Text != "Let me check." {
		t.Errorf("expected text 'Let me check.', got %q", textEvents[0].Text)
	}

	if len(toolStarts) != 1 {
		t.Fatalf("expected 1 tool start, got %d", len(toolStarts))
	}
	if toolStarts[0].ID != "call_s1" {
		t.Errorf("expected tool ID 'call_s1', got %q", toolStarts[0].ID)
	}
	if toolStarts[0].Name != "file_read" {
		t.Errorf("expected tool name 'file_read', got %q", toolStarts[0].Name)
	}

	if len(toolEnds) != 1 {
		t.Fatalf("expected 1 tool end, got %d", len(toolEnds))
	}
	if string(toolEnds[0].Input) != `{"path":"auth.go"}` {
		t.Errorf("expected tool input %q, got %q", `{"path":"auth.go"}`, string(toolEnds[0].Input))
	}

	if len(doneEvents) != 1 {
		t.Fatalf("expected 1 done event, got %d", len(doneEvents))
	}
	if doneEvents[0].StopReason != provider.StopReasonToolUse {
		t.Errorf("expected StopReasonToolUse, got %q", doneEvents[0].StopReason)
	}
}

func TestIntegration_SSEStreamingErrorOnConnect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
		fmt.Fprint(w, `{"error":{"message":"Service unavailable"}}`)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL, "test-key")
	ch, err := p.Stream(context.Background(), &provider.Request{
		Messages: []provider.Message{provider.NewUserMessage("hello")},
	})
	if ch != nil {
		t.Error("expected nil channel")
	}
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "server error (HTTP 503)") {
		t.Errorf("expected server error message, got %q", err.Error())
	}
}

func TestIntegration_PlainTextResponseWhenToolsRequested(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id": "chatcmpl-notool",
			"object": "chat.completion",
			"model": "test-model",
			"choices": [
				{
					"index": 0,
					"message": {"role": "assistant", "content": "I cannot use tools but here is my answer."},
					"finish_reason": "stop"
				}
			],
			"usage": {"prompt_tokens": 50, "completion_tokens": 20, "total_tokens": 70}
		}`)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL, "test-key")
	resp, err := p.Complete(context.Background(), &provider.Request{
		Messages: []provider.Message{provider.NewUserMessage("read a file")},
		Tools: []provider.ToolDefinition{
			{
				Name:        "file_read",
				Description: "Read a file",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(resp.Content))
	}
	if resp.Content[0].Type != "text" {
		t.Errorf("expected text block, got %q", resp.Content[0].Type)
	}
	if resp.StopReason != provider.StopReasonEndTurn {
		t.Errorf("expected StopReasonEndTurn, got %q", resp.StopReason)
	}
}

func TestIntegration_ConnectionRefused(t *testing.T) {
	// Get a free port and then close it so nothing is listening.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to get free port: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	p, err := NewOpenAIProvider(OpenAIConfig{
		Name:          "test",
		BaseURL:       "http://" + addr,
		Model:         "test-model",
		ContextLength: 4096,
	})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	_, completeErr := p.Complete(context.Background(), &provider.Request{
		Messages: []provider.Message{provider.NewUserMessage("hello")},
	})
	if completeErr == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(completeErr.Error(), "is not reachable. Is the model server running?") {
		t.Errorf("expected connection refused error, got %q", completeErr.Error())
	}
}
