package server_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"nhooyr.io/websocket"

	"github.com/ponchione/sirtopham/internal/agent"
	"github.com/ponchione/sirtopham/internal/conversation"
	"github.com/ponchione/sirtopham/internal/server"
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
	convMock := &mockConversationService{}
	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0, DevMode: true}, newTestLogger())
	server.NewWebSocketHandler(srv, agentMock, convMock, "test-project", newTestLogger())
	_, base := startServer(t, srv)
	return base, convMock
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

	var resp map[string]any
	json.Unmarshal(respData, &resp)
	if resp["type"] != "error" {
		t.Fatalf("expected error response, got %v", resp["type"])
	}

	// Unblock.
	close(blockCh)

	conn.Close(websocket.StatusNormalClosure, "test done")
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

	// The token event type constant is "***" (from events.go).
	if resp["type"] != "token" {
		// Could also be "***" depending on the EventType() implementation.
		// Accept either.
		if resp["type"] != "***" {
			t.Fatalf("expected token event, got type=%v", resp["type"])
		}
	}

	conn.Close(websocket.StatusNormalClosure, "test done")
}

// Ensure the mock satisfies the interfaces.
var _ server.AgentService = (*mockAgentService)(nil)
var _ server.ConversationService = (*mockConversationService)(nil)
var _ = (*conversation.Conversation)(nil) // keep import used
