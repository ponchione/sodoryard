package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"nhooyr.io/websocket"

	"github.com/ponchione/sodoryard/internal/agent"
	"github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/conversation"
	"github.com/ponchione/sodoryard/internal/provider"
	"github.com/ponchione/sodoryard/internal/server"
)

// mockAgentService is a test double implementing server.AgentService.
type mockAgentService struct {
	mu          sync.Mutex
	runTurnFn   func(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error)
	cancelCount int
	subscribed  []agent.EventSink
}

func (m *mockAgentService) RunTurn(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
	if m.runTurnFn != nil {
		return m.runTurnFn(ctx, req)
	}
	return &agent.TurnResult{}, nil
}

func (m *mockAgentService) Subscribe(sink agent.EventSink) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subscribed = append(m.subscribed, sink)
}

func (m *mockAgentService) Unsubscribe(sink agent.EventSink) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, s := range m.subscribed {
		if s == sink {
			m.subscribed = append(m.subscribed[:i], m.subscribed[i+1:]...)
			break
		}
	}
}

func (m *mockAgentService) Cancel() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancelCount++
}

func (m *mockAgentService) getCancelCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cancelCount
}

func (m *mockAgentService) getSubscribed() []agent.EventSink {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]agent.EventSink, len(m.subscribed))
	copy(cp, m.subscribed)
	return cp
}

func setupWSTest(t *testing.T, agentMock *mockAgentService) (string, *mockConversationService) {
	t.Helper()
	conv123 := &conversation.Conversation{ID: "conv-123", ProjectID: "test-project", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	conv1 := &conversation.Conversation{ID: "conv-1", ProjectID: "test-project", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	convMock := &mockConversationService{conversations: []*conversation.Conversation{conv123, conv1}}
	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0, DevMode: true}, newTestLogger())
	cfg := &config.Config{ProjectRoot: "test-project", Providers: map[string]config.ProviderConfig{
		"codex": {Type: "codex", ContextLength: 200000},
		"local": {Type: "openai-compatible", ContextLength: 32768},
	}}
	server.NewWebSocketHandler(srv, agentMock, convMock, cfg, nil, newTestLogger())
	_, base := startServer(t, srv)
	return base, convMock
}

func TestWebSocketDoesNotLogExpectedCancellationAsRunTurnError(t *testing.T) {
	testLogBuf := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(testLogBuf, nil))
	agentMock := &mockAgentService{
		runTurnFn: func(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
			return nil, fmt.Errorf("%w: %v", agent.ErrTurnCancelled, context.Canceled)
		},
	}
	conv123 := &conversation.Conversation{ID: "conv-123", ProjectID: "test-project", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	convMock := &mockConversationService{conversations: []*conversation.Conversation{conv123}}
	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0, DevMode: true}, logger)
	cfg := &config.Config{ProjectRoot: "test-project", Providers: map[string]config.ProviderConfig{
		"codex": {Type: "codex", ContextLength: 200000},
	}}
	server.NewWebSocketHandler(srv, agentMock, convMock, cfg, nil, logger)
	_, base := startServer(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + base[4:] + "/api/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer conn.CloseNow()

	msg := map[string]string{
		"type":            "message",
		"conversation_id": "conv-123",
		"content":         "cancel me",
	}
	data, _ := json.Marshal(msg)
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	logOutput := testLogBuf.String()
	if strings.Contains(logOutput, "level=ERROR msg=\"run turn\"") {
		t.Fatalf("expected no run-turn error log for expected cancellation, got logs:\n%s", logOutput)
	}
}

func TestWebSocketUpgrade(t *testing.T) {
	agentMock := &mockAgentService{}
	base, _ := setupWSTest(t, agentMock)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Connect WebSocket.
	wsURL := "ws" + base[4:] + "/api/ws" // http:// → ws://
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer conn.CloseNow()

	conn.Close(websocket.StatusNormalClosure, "test done")
}

func TestWebSocketMessageTriggersRunTurn(t *testing.T) {
	turnStarted := make(chan agent.RunTurnRequest, 1)
	agentMock := &mockAgentService{
		runTurnFn: func(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
			turnStarted <- req
			return &agent.TurnResult{}, nil
		},
	}
	base, _ := setupWSTest(t, agentMock)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + base[4:] + "/api/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer conn.CloseNow()

	// Send a message.
	msg := map[string]string{
		"type":            "message",
		"conversation_id": "conv-123",
		"content":         "Hello agent",
	}
	data, _ := json.Marshal(msg)
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// Wait for RunTurn to be called.
	select {
	case req := <-turnStarted:
		if req.ConversationID != "conv-123" {
			t.Fatalf("expected conversation_id conv-123, got %q", req.ConversationID)
		}
		if req.Message != "Hello agent" {
			t.Fatalf("expected message 'Hello agent', got %q", req.Message)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for RunTurn")
	}

	conn.Close(websocket.StatusNormalClosure, "test done")
}

func TestWebSocketMessageUsesNextTurnNumberForNewConversation(t *testing.T) {
	turnStarted := make(chan agent.RunTurnRequest, 1)
	agentMock := &mockAgentService{
		runTurnFn: func(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
			turnStarted <- req
			return &agent.TurnResult{}, nil
		},
	}
	base, _ := setupWSTest(t, agentMock)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + base[4:] + "/api/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer conn.CloseNow()

	msg := map[string]string{
		"type":    "message",
		"content": "New chat",
	}
	data, _ := json.Marshal(msg)
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	_, respData, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	var resp map[string]any
	json.Unmarshal(respData, &resp)
	if resp["type"] != "conversation_created" {
		t.Fatalf("expected conversation_created, got %v", resp["type"])
	}

	select {
	case req := <-turnStarted:
		if req.ConversationID == "" {
			t.Fatal("expected non-empty conversation_id after auto-create")
		}
		if req.TurnNumber != 1 {
			t.Fatalf("TurnNumber = %d, want 1", req.TurnNumber)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for RunTurn")
	}

	conn.Close(websocket.StatusNormalClosure, "test done")
}

func TestWebSocketMessageCreatesConversation(t *testing.T) {
	turnStarted := make(chan agent.RunTurnRequest, 1)
	agentMock := &mockAgentService{
		runTurnFn: func(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
			turnStarted <- req
			return &agent.TurnResult{}, nil
		},
	}
	base, _ := setupWSTest(t, agentMock)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + base[4:] + "/api/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer conn.CloseNow()

	// Send a message without conversation_id — should auto-create.
	msg := map[string]string{
		"type":    "message",
		"content": "New chat",
	}
	data, _ := json.Marshal(msg)
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// Read the conversation_created response.
	_, respData, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	var resp map[string]any
	json.Unmarshal(respData, &resp)
	if resp["type"] != "conversation_created" {
		t.Fatalf("expected conversation_created, got %v", resp["type"])
	}

	// RunTurn should be called with the new conversation ID.
	select {
	case req := <-turnStarted:
		if req.ConversationID == "" {
			t.Fatal("expected non-empty conversation_id after auto-create")
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for RunTurn")
	}

	conn.Close(websocket.StatusNormalClosure, "test done")
}

func TestWebSocketNewConversationPersistsResolvedRuntimeDefaults(t *testing.T) {
	turnStarted := make(chan agent.RunTurnRequest, 1)
	agentMock := &mockAgentService{runTurnFn: func(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
		turnStarted <- req
		return &agent.TurnResult{}, nil
	}}
	base, convMock := setupWSTest(t, agentMock)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wsURL := "ws" + base[4:] + "/api/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer conn.CloseNow()

	msg := map[string]string{"type": "message", "content": "New chat", "provider": "local", "model": "qwen-coder"}
	data, _ := json.Marshal(msg)
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if _, _, err := conn.Read(ctx); err != nil {
		t.Fatalf("read conversation_created failed: %v", err)
	}
	select {
	case req := <-turnStarted:
		if req.Provider != "codex" || req.Model != "gpt-5.5" {
			t.Fatalf("RunTurn provider/model = %q/%q, want codex/gpt-5.5", req.Provider, req.Model)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for RunTurn")
	}
	createOpts := &conversation.CreateOptions{}
	for _, opt := range convMock.lastCreateOpts {
		opt(createOpts)
	}
	if createOpts.Provider == nil || *createOpts.Provider != "codex" {
		t.Fatalf("created provider default = %#v, want codex", createOpts.Provider)
	}
	if createOpts.Model == nil || *createOpts.Model != "gpt-5.5" {
		t.Fatalf("created model default = %#v, want gpt-5.5", createOpts.Model)
	}
}

func TestWebSocketExistingConversationUsesStoredRuntimeDefaults(t *testing.T) {
	turnStarted := make(chan agent.RunTurnRequest, 1)
	agentMock := &mockAgentService{runTurnFn: func(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
		turnStarted <- req
		return &agent.TurnResult{}, nil
	}}
	base, convMock := setupWSTest(t, agentMock)
	providerName := "local"
	modelName := "qwen-coder"
	convMock.conversations = []*conversation.Conversation{{ID: "conv-1", ProjectID: "test-project", Provider: &providerName, Model: &modelName, CreatedAt: time.Now(), UpdatedAt: time.Now()}}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wsURL := "ws" + base[4:] + "/api/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer conn.CloseNow()

	msg := map[string]string{"type": "message", "conversation_id": "conv-1", "content": "hello"}
	data, _ := json.Marshal(msg)
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	select {
	case req := <-turnStarted:
		if req.Provider != "codex" || req.Model != "gpt-5.5" {
			t.Fatalf("RunTurn provider/model = %q/%q, want codex/gpt-5.5", req.Provider, req.Model)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for RunTurn")
	}
	if convMock.lastSetID != "conv-1" {
		t.Fatalf("persisted runtime defaults for conversation = %q, want conv-1", convMock.lastSetID)
	}
	if convMock.lastSetProvider == nil || *convMock.lastSetProvider != "codex" {
		t.Fatalf("persisted provider default = %#v, want codex", convMock.lastSetProvider)
	}
	if convMock.lastSetModel == nil || *convMock.lastSetModel != "gpt-5.5" {
		t.Fatalf("persisted model default = %#v, want gpt-5.5", convMock.lastSetModel)
	}
}

func TestWebSocketUsesRoutingDefaultProvider(t *testing.T) {
	turnStarted := make(chan agent.RunTurnRequest, 1)
	agentMock := &mockAgentService{
		runTurnFn: func(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
			turnStarted <- req
			return &agent.TurnResult{}, nil
		},
	}
	convMock := &mockConversationService{}
	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0, DevMode: true}, newTestLogger())
	cfg := &config.Config{
		ProjectRoot: "test-project",
		Routing:     config.RoutingConfig{Default: config.RouteConfig{Provider: "anthropic", Model: "claude-sonnet-4-6-20250514"}},
		Providers: map[string]config.ProviderConfig{
			"anthropic": {Type: "anthropic", ContextLength: 200000},
			"codex":     {Type: "codex", ContextLength: 200000},
		},
	}
	server.NewWebSocketHandler(srv, agentMock, convMock, cfg, nil, newTestLogger())
	_, base := startServer(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + base[4:] + "/api/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer conn.CloseNow()

	msg := map[string]string{"type": "message", "content": "hello"}
	data, _ := json.Marshal(msg)
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	_, _, err = conn.Read(ctx) // conversation_created
	if err != nil {
		t.Fatalf("read conversation_created failed: %v", err)
	}

	select {
	case req := <-turnStarted:
		if req.Provider != "codex" {
			t.Fatalf("Provider = %q, want codex", req.Provider)
		}
		if req.Model != "gpt-5.5" {
			t.Fatalf("Model = %q, want gpt-5.5", req.Model)
		}
		if req.ModelContextLimit != 200000 {
			t.Fatalf("ModelContextLimit = %d, want 200000", req.ModelContextLimit)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for RunTurn")
	}

	conn.Close(websocket.StatusNormalClosure, "test done")
}

func TestWebSocketIgnoresConfigAPIOverrideAndStaysOnForcedCodexGPT55(t *testing.T) {
	turnStarted := make(chan agent.RunTurnRequest, 1)
	agentMock := &mockAgentService{
		runTurnFn: func(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
			turnStarted <- req
			return &agent.TurnResult{}, nil
		},
	}
	convMock := &mockConversationService{}
	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0, DevMode: true}, newTestLogger())
	cfg := &config.Config{
		ProjectRoot: "test-project",
		Routing:     config.RoutingConfig{Default: config.RouteConfig{Provider: "codex", Model: "gpt-5.5"}},
		Providers: map[string]config.ProviderConfig{
			"codex": {Type: "codex", Model: "gpt-5.5", ContextLength: 200000},
			"local": {Type: "openai-compatible", Model: "new-runtime-model", ContextLength: 32768},
		},
	}
	runtime := &stubRuntimeInspector{models: []provider.Model{{ID: "gpt-5.5", Provider: "codex"}, {ID: "new-runtime-model", Provider: "local"}}}
	defaults := server.NewRuntimeDefaults(cfg)
	server.NewConfigHandler(srv, cfg, runtime, defaults, newTestLogger())
	server.NewWebSocketHandler(srv, agentMock, convMock, cfg, defaults, newTestLogger())
	_, base := startServer(t, srv)

	updateBody := []byte(`{"default_provider":"local","default_model":"new-runtime-model"}`)
	req, err := http.NewRequest(http.MethodPut, base+"/api/config", bytes.NewReader(updateBody))
	if err != nil {
		t.Fatalf("new PUT request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /api/config failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("PUT /api/config status = %d, want 400", resp.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + base[4:] + "/api/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer conn.CloseNow()

	msg := map[string]string{"type": "message", "content": "hello"}
	data, _ := json.Marshal(msg)
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	_, _, err = conn.Read(ctx)
	if err != nil {
		t.Fatalf("read conversation_created failed: %v", err)
	}

	select {
	case runReq := <-turnStarted:
		if runReq.Provider != "codex" {
			t.Fatalf("Provider = %q, want codex", runReq.Provider)
		}
		if runReq.Model != "gpt-5.5" {
			t.Fatalf("Model = %q, want gpt-5.5", runReq.Model)
		}
		if runReq.ModelContextLimit != 200000 {
			t.Fatalf("ModelContextLimit = %d, want 200000", runReq.ModelContextLimit)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for RunTurn")
	}
}

func TestWebSocketIgnoresPerConnectionModelOverrideAndStaysOnForcedCodexGPT55(t *testing.T) {
	turnStarted := make(chan agent.RunTurnRequest, 1)
	agentMock := &mockAgentService{
		runTurnFn: func(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
			turnStarted <- req
			return &agent.TurnResult{}, nil
		},
	}
	convMock := &mockConversationService{}
	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0, DevMode: true}, newTestLogger())
	cfg := &config.Config{ProjectRoot: "test-project", Routing: config.RoutingConfig{Default: config.RouteConfig{Provider: "codex", Model: "gpt-5.5"}}, Providers: map[string]config.ProviderConfig{
		"codex": {Type: "codex", Model: "gpt-5.5", ContextLength: 200000},
		"local": {Type: "openai-compatible", Model: "other-model", ContextLength: 32768},
	}}
	server.NewWebSocketHandler(srv, agentMock, convMock, cfg, server.NewRuntimeDefaults(cfg), newTestLogger())
	_, base := startServer(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wsURL := "ws" + base[4:] + "/api/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer conn.CloseNow()

	overrideMsg := map[string]string{"type": "model_override", "provider": "local", "model": "other-model"}
	overrideData, _ := json.Marshal(overrideMsg)
	if err := conn.Write(ctx, websocket.MessageText, overrideData); err != nil {
		t.Fatalf("write override failed: %v", err)
	}

	msg := map[string]string{"type": "message", "content": "hello"}
	data, _ := json.Marshal(msg)
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	_, _, err = conn.Read(ctx)
	if err != nil {
		t.Fatalf("read conversation_created failed: %v", err)
	}

	select {
	case runReq := <-turnStarted:
		if runReq.Provider != "codex" {
			t.Fatalf("Provider = %q, want codex", runReq.Provider)
		}
		if runReq.Model != "gpt-5.5" {
			t.Fatalf("Model = %q, want gpt-5.5", runReq.Model)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for RunTurn")
	}
}

func TestWebSocketUsesFallbackContextLimitForCodexWithoutConfiguredLength(t *testing.T) {
	turnStarted := make(chan agent.RunTurnRequest, 1)
	agentMock := &mockAgentService{
		runTurnFn: func(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
			turnStarted <- req
			return &agent.TurnResult{}, nil
		},
	}
	convMock := &mockConversationService{}
	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0, DevMode: true}, newTestLogger())
	cfg := &config.Config{ProjectRoot: "test-project", Providers: map[string]config.ProviderConfig{
		"codex": {Type: "codex"},
	}}
	server.NewWebSocketHandler(srv, agentMock, convMock, cfg, nil, newTestLogger())
	_, base := startServer(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + base[4:] + "/api/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer conn.CloseNow()

	msg := map[string]string{"type": "message", "content": "hello"}
	data, _ := json.Marshal(msg)
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	_, _, err = conn.Read(ctx) // conversation_created
	if err != nil {
		t.Fatalf("read conversation_created failed: %v", err)
	}

	select {
	case req := <-turnStarted:
		if req.Provider != "codex" {
			t.Fatalf("Provider = %q, want codex", req.Provider)
		}
		if req.ModelContextLimit != 200000 {
			t.Fatalf("ModelContextLimit = %d, want 200000", req.ModelContextLimit)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for RunTurn")
	}

	conn.Close(websocket.StatusNormalClosure, "test done")
}

func TestWebSocketMessageUsesNextTurnNumberForExistingConversation(t *testing.T) {
	turnStarted := make(chan agent.RunTurnRequest, 1)
	agentMock := &mockAgentService{
		runTurnFn: func(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
			turnStarted <- req
			return &agent.TurnResult{}, nil
		},
	}
	base, convMock := setupWSTest(t, agentMock)
	content := "previous"
	convMock.messages = []conversation.MessageView{
		{ID: 1, Role: "user", Content: &content, TurnNumber: 1, Sequence: 0},
		{ID: 2, Role: "assistant", Content: &content, TurnNumber: 1, Sequence: 1},
		{ID: 3, Role: "user", Content: &content, TurnNumber: 2, Sequence: 2},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + base[4:] + "/api/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer conn.CloseNow()

	msg := map[string]string{"type": "message", "conversation_id": "conv-1", "content": "hi again"}
	data, _ := json.Marshal(msg)
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	select {
	case req := <-turnStarted:
		if req.TurnNumber != 3 {
			t.Fatalf("TurnNumber = %d, want 3", req.TurnNumber)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for RunTurn")
	}

	conn.Close(websocket.StatusNormalClosure, "test done")
}

func TestWebSocketCancel(t *testing.T) {
	// RunTurn blocks so we can test cancel.
	blockCh := make(chan struct{})
	agentMock := &mockAgentService{
		runTurnFn: func(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
			<-blockCh // Block until we unblock
			return &agent.TurnResult{}, nil
		},
	}
	base, _ := setupWSTest(t, agentMock)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + base[4:] + "/api/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer conn.CloseNow()

	// Start a turn.
	msg := map[string]string{"type": "message", "conversation_id": "conv-1", "content": "hi"}
	data, _ := json.Marshal(msg)
	conn.Write(ctx, websocket.MessageText, data)

	// Give it time to start.
	time.Sleep(100 * time.Millisecond)

	// Send cancel.
	cancelMsg := map[string]string{"type": "cancel"}
	data, _ = json.Marshal(cancelMsg)
	conn.Write(ctx, websocket.MessageText, data)

	time.Sleep(50 * time.Millisecond)

	if agentMock.getCancelCount() == 0 {
		t.Fatal("expected Cancel() to be called")
	}

	// Unblock the turn so cleanup can proceed.
	close(blockCh)

	conn.Close(websocket.StatusNormalClosure, "test done")
}

func TestWebSocketDisconnectCancelsTurnContext(t *testing.T) {
	turnStarted := make(chan struct{})
	ctxCancelled := make(chan struct{})
	agentMock := &mockAgentService{
		runTurnFn: func(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
			close(turnStarted)
			<-ctx.Done()
			close(ctxCancelled)
			return nil, ctx.Err()
		},
	}
	base, _ := setupWSTest(t, agentMock)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + base[4:] + "/api/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}

	msg := map[string]string{"type": "message", "conversation_id": "conv-1", "content": "hi"}
	data, _ := json.Marshal(msg)
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	select {
	case <-turnStarted:
	case <-ctx.Done():
		t.Fatal("timed out waiting for turn start")
	}

	if err := conn.Close(websocket.StatusNormalClosure, "disconnect"); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	select {
	case <-ctxCancelled:
	case <-ctx.Done():
		t.Fatal("timed out waiting for RunTurn context cancellation after websocket disconnect")
	}

	if agentMock.getCancelCount() != 0 {
		t.Fatal("expected websocket disconnect to cancel via context, not explicit Cancel()")
	}
}

func TestWebSocketOneTurnAtATime(t *testing.T) {
	blockCh := make(chan struct{})
	agentMock := &mockAgentService{
		runTurnFn: func(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
			<-blockCh
			return &agent.TurnResult{}, nil
		},
	}
	base, _ := setupWSTest(t, agentMock)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + base[4:] + "/api/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer conn.CloseNow()

	// Start a turn.
	msg1 := map[string]string{"type": "message", "conversation_id": "conv-1", "content": "first"}
	data, _ := json.Marshal(msg1)
	conn.Write(ctx, websocket.MessageText, data)

	time.Sleep(100 * time.Millisecond)

	// Try a second turn — should get an error.
	msg2 := map[string]string{"type": "message", "conversation_id": "conv-1", "content": "second"}
	data, _ = json.Marshal(msg2)
	conn.Write(ctx, websocket.MessageText, data)

	// Read the error response.
	_, respData, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	var resp struct {
		Type string         `json:"type"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(respData, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Type != "error" {
		t.Fatalf("expected error response, got %v", resp.Type)
	}
	if got := resp.Data["message"]; got != "a turn is already in progress" {
		t.Fatalf("error message = %v, want %q", got, "a turn is already in progress")
	}
	if got := resp.Data["recoverable"]; got != true {
		t.Fatalf("recoverable = %v, want true", got)
	}
	if got := resp.Data["error_code"]; got != "turn_in_progress" {
		t.Fatalf("error_code = %v, want %q", got, "turn_in_progress")
	}
	if _, ok := resp.Data["error"]; ok {
		t.Fatalf("unexpected legacy error field in payload: %#v", resp.Data)
	}

	// Unblock.
	close(blockCh)

	conn.Close(websocket.StatusNormalClosure, "test done")
}

func TestWebSocketInvalidJSONUsesMessageErrorPayload(t *testing.T) {
	agentMock := &mockAgentService{}
	base, _ := setupWSTest(t, agentMock)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + base[4:] + "/api/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer conn.CloseNow()

	if err := conn.Write(ctx, websocket.MessageText, []byte("{not json")); err != nil {
		t.Fatalf("write invalid json failed: %v", err)
	}

	_, respData, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	var resp struct {
		Type string         `json:"type"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(respData, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Type != "error" {
		t.Fatalf("expected error response, got %v", resp.Type)
	}
	if got := resp.Data["message"]; got != "invalid message" {
		t.Fatalf("error message = %v, want %q", got, "invalid message")
	}
	if _, ok := resp.Data["recoverable"]; ok {
		t.Fatalf("recoverable should be omitted for false values: %#v", resp.Data)
	}
	if got := resp.Data["error_code"]; got != "invalid_message" {
		t.Fatalf("error_code = %v, want %q", got, "invalid_message")
	}
	if _, ok := resp.Data["error"]; ok {
		t.Fatalf("unexpected legacy error field in payload: %#v", resp.Data)
	}
}

func TestWebSocketEventForwarding(t *testing.T) {
	agentMock := &mockAgentService{}
	agentMock.runTurnFn = func(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
		// Emit an event through the subscribed sinks.
		time.Sleep(50 * time.Millisecond) // Let subscribe happen first
		for _, sink := range agentMock.getSubscribed() {
			sink.Emit(agent.TokenEvent{
				Type:  "token",
				Token: "Hello",
				Time:  time.Now(),
			})
		}
		return &agent.TurnResult{}, nil
	}
	base, _ := setupWSTest(t, agentMock)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + base[4:] + "/api/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer conn.CloseNow()

	// Send message.
	msg := map[string]string{"type": "message", "conversation_id": "conv-1", "content": "hi"}
	data, _ := json.Marshal(msg)
	conn.Write(ctx, websocket.MessageText, data)

	// Read the forwarded token event.
	_, respData, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	var resp map[string]any
	json.Unmarshal(respData, &resp)

	if resp["type"] != "token" {
		t.Fatalf("expected token event, got type=%v", resp["type"])
	}
	dataMap, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("resp[data] type = %T, want object", resp["data"])
	}
	if dataMap["token"] != "Hello" {
		t.Fatalf("forwarded token = %v, want Hello", dataMap["token"])
	}

	conn.Close(websocket.StatusNormalClosure, "test done")
}

// Ensure the mock satisfies the interfaces.
var _ server.AgentService = (*mockAgentService)(nil)
var _ server.ConversationService = (*mockConversationService)(nil)
var _ = (*conversation.Conversation)(nil) // keep import used
