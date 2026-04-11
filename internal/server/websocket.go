package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"nhooyr.io/websocket"

	"github.com/ponchione/sodoryard/internal/agent"
	"github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/conversation"
)

// AgentService is the interface the WebSocket handler needs from the agent loop.
type AgentService interface {
	RunTurn(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error)
	Subscribe(sink agent.EventSink)
	Unsubscribe(sink agent.EventSink)
	Cancel()
}

// connOverride holds per-connection model/provider overrides set by the
// "model_override" client event. Guarded by mu for goroutine safety between
// the readLoop and handleMessage goroutines.
type connOverride struct {
	mu       sync.Mutex
	model    string
	provider string
}

// WebSocketHandler handles WebSocket connections for streaming agent events.
type WebSocketHandler struct {
	agent     AgentService
	convSvc   ConversationService
	projectID string
	providers map[string]config.ProviderConfig
	defaults  *RuntimeDefaults
	devMode   bool
	logger    *slog.Logger
}

// NewWebSocketHandler creates a handler and registers the WS route.
func NewWebSocketHandler(s *Server, agentSvc AgentService, convSvc ConversationService, cfg *config.Config, defaults *RuntimeDefaults, logger *slog.Logger) *WebSocketHandler {
	if defaults == nil {
		defaults = NewRuntimeDefaults(cfg)
	}
	h := &WebSocketHandler{
		agent:     agentSvc,
		convSvc:   convSvc,
		projectID: cfg.ProjectRoot,
		providers: cfg.Providers,
		defaults:  defaults,
		devMode:   cfg.Server.DevMode,
		logger:    logger,
	}
	s.HandleFunc("/api/ws", h.handleWS)
	return h
}

// ClientMessage represents a message from the WebSocket client.
type ClientMessage struct {
	Type           string `json:"type"`
	ConversationID string `json:"conversation_id,omitempty"`
	Content        string `json:"content,omitempty"`
	Model          string `json:"model,omitempty"`
	Provider       string `json:"provider,omitempty"`
}

// ServerMessage is the envelope for events sent to the WebSocket client.
type ServerMessage struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Data      any       `json:"data"`
}

func (h *WebSocketHandler) defaultProviderName() string {
	if h.defaults != nil {
		if provider, _ := h.defaults.Get(); provider != "" {
			return provider
		}
	}
	for name := range h.providers {
		if name == "codex" {
			return name
		}
	}
	for name := range h.providers {
		return name
	}
	return ""
}

func (h *WebSocketHandler) handleWS(w http.ResponseWriter, r *http.Request) {
	acceptOptions := &websocket.AcceptOptions{}
	if h.devMode {
		// In dev mode, Vite dev server connects from a different origin.
		acceptOptions.InsecureSkipVerify = true
	}
	conn, err := websocket.Accept(w, r, acceptOptions)
	if err != nil {
		h.logger.Error("websocket accept failed", "error", err)
		return
	}
	defer conn.CloseNow()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	h.logger.Info("websocket connected", "remote", r.RemoteAddr)

	// Create a channel sink for this connection.
	sink := agent.NewChannelSink(256)

	// Track whether a turn is in progress.
	var turnActive atomic.Bool

	// Per-connection model/provider override.
	override := &connOverride{}

	// Read loop goroutine (client → server).
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		h.readLoop(ctx, cancel, conn, sink, &turnActive, override)
	}()

	// Write loop (server → client) — blocks until ctx done or sink closed.
	h.writeLoop(ctx, conn, sink)
	// If the write side exits first (for example because the client disconnected
	// and writes start failing), cancel the shared turn context so any in-flight
	// RunTurn call does not wait indefinitely for the read loop to notice.
	cancel()

	// Wait for read loop to finish.
	<-readDone
	h.logger.Info("websocket disconnected", "remote", r.RemoteAddr)
}

// writeLoop sends events and heartbeats to the WebSocket client.
func (h *WebSocketHandler) writeLoop(ctx context.Context, conn *websocket.Conn, sink *agent.ChannelSink) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	events := sink.Events()

	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-events:
			if !ok {
				return
			}
			msg := ServerMessage{
				Type:      evt.EventType(),
				Timestamp: evt.Timestamp(),
				Data:      evt,
			}
			data, err := json.Marshal(msg)
			if err != nil {
				h.logger.Error("marshal event", "error", err, "type", evt.EventType())
				continue
			}
			writeCtx, writeCancel := context.WithTimeout(ctx, 5*time.Second)
			err = conn.Write(writeCtx, websocket.MessageText, data)
			writeCancel()
			if err != nil {
				h.logger.Debug("websocket write error", "error", err)
				return
			}
		case <-ticker.C:
			pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
			err := conn.Ping(pingCtx)
			pingCancel()
			if err != nil {
				h.logger.Debug("websocket ping failed", "error", err)
				return
			}
		}
	}
}

// readLoop reads client messages and dispatches them.
func (h *WebSocketHandler) readLoop(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, sink *agent.ChannelSink, turnActive *atomic.Bool, override *connOverride) {
	defer cancel()

	var turnWg sync.WaitGroup

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			// Normal close or context cancelled.
			h.logger.Debug("websocket read error", "error", err)
			break
		}

		var msg ClientMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			h.logger.Warn("invalid websocket message", "error", err)
			h.writeJSONMessage(ctx, conn, "error", map[string]string{"error": "invalid message"})
			continue
		}

		switch msg.Type {
		case "message":
			if !turnActive.CompareAndSwap(false, true) {
				h.writeJSONMessage(ctx, conn, "error", map[string]string{"error": "a turn is already in progress"})
				continue
			}

			turnWg.Add(1)
			go func() {
				defer turnWg.Done()
				defer turnActive.Store(false)
				h.handleMessage(ctx, conn, sink, msg, override)
			}()

		case "model_override":
			// Store the model/provider override for the next "message" turn.
			override.mu.Lock()
			override.model = msg.Model
			override.provider = msg.Provider
			override.mu.Unlock()
			h.logger.Info("model override set",
				"model", msg.Model,
				"provider", msg.Provider,
			)

		case "cancel":
			h.agent.Cancel()

		default:
			h.logger.Warn("unknown client message type", "type", msg.Type)
		}
	}

	// Wait for any in-progress turn to finish before returning.
	// Disconnects cancel the shared turn context; unsubscribing the sink here
	// stops any further event forwarding during teardown.
	h.agent.Unsubscribe(sink)
	sink.Close()
	turnWg.Wait()
}

// handleMessage processes a "message" client command — creates or resumes a
// conversation and runs a turn.
func (h *WebSocketHandler) nextTurnNumber(ctx context.Context, conversationID string) (int, error) {
	messages, err := h.convSvc.GetMessages(ctx, conversationID)
	if err != nil {
		return 0, err
	}
	maxTurn := 0
	for _, msg := range messages {
		if int(msg.TurnNumber) > maxTurn {
			maxTurn = int(msg.TurnNumber)
		}
	}
	return maxTurn + 1, nil
}

func (h *WebSocketHandler) resolveModelContextLimit(providerName string) (int, error) {
	if providerName == "" {
		return 0, fmt.Errorf("provider name is required")
	}
	cfg, ok := h.providers[providerName]
	if !ok {
		return 0, fmt.Errorf("unknown provider: %s", providerName)
	}
	if cfg.ContextLength > 0 {
		return cfg.ContextLength, nil
	}
	switch cfg.Type {
	case "anthropic", "codex":
		return 200000, nil
	case "openai-compatible":
		return 32768, nil
	default:
		return 0, fmt.Errorf("provider %s has no positive context_length configured", providerName)
	}
}

func (h *WebSocketHandler) handleMessage(ctx context.Context, conn *websocket.Conn, sink *agent.ChannelSink, msg ClientMessage, override *connOverride) {
	// Subscribe sink to receive events for this turn.
	h.agent.Subscribe(sink)
	defer h.agent.Unsubscribe(sink)

	convID := msg.ConversationID

	// Resolve model/provider: prefer inline fields on the message, then the
	// stored override from a preceding "model_override" event, then persisted
	// conversation defaults, and finally runtime defaults.
	model := msg.Model
	prov := msg.Provider
	override.mu.Lock()
	if model == "" {
		model = override.model
	}
	if prov == "" {
		prov = override.provider
	}
	// Clear the stored override so it only applies to one turn.
	override.model = ""
	override.provider = ""
	override.mu.Unlock()

	var conversationDefaults *conversation.Conversation
	if convID != "" {
		stored, err := h.convSvc.Get(ctx, convID)
		if err != nil {
			h.logger.Error("load conversation defaults", "error", err, "conversation_id", convID)
			h.writeJSONMessage(ctx, conn, "error", map[string]string{"error": "failed to load conversation"})
			return
		}
		conversationDefaults = stored
		if prov == "" && stored.Provider != nil {
			prov = *stored.Provider
		}
		if model == "" && stored.Model != nil {
			model = *stored.Model
		}
	}

	defaultProvider, defaultModel := h.defaults.Get()
	if defaultProvider == "" {
		defaultProvider = h.defaultProviderName()
	}
	if prov == "" {
		prov = defaultProvider
	}
	if model == "" && prov == defaultProvider {
		model = defaultModel
	}

	if convID == "" {
		var opts []conversation.CreateOption
		if prov != "" {
			opts = append(opts, conversation.WithProvider(prov))
		}
		if model != "" {
			opts = append(opts, conversation.WithModel(model))
		}
		c, err := h.convSvc.Create(ctx, h.projectID, opts...)
		if err != nil {
			h.logger.Error("create conversation for ws", "error", err)
			h.writeJSONMessage(ctx, conn, "error", map[string]string{"error": "failed to create conversation"})
			return
		}
		convID = c.ID
		h.writeJSONMessage(ctx, conn, "conversation_created", map[string]string{"conversation_id": convID})
	} else if conversationDefaults != nil {
		providerChanged := conversationDefaults.Provider == nil || *conversationDefaults.Provider != prov
		modelChanged := conversationDefaults.Model == nil || *conversationDefaults.Model != model
		if providerChanged || modelChanged {
			if err := h.convSvc.SetRuntimeDefaults(ctx, convID, &prov, &model); err != nil {
				h.logger.Error("persist conversation defaults", "error", err, "conversation_id", convID)
				h.writeJSONMessage(ctx, conn, "error", map[string]string{"error": "failed to persist conversation defaults"})
				return
			}
		}
	}

	turnNumber, turnErr := h.nextTurnNumber(ctx, convID)
	if turnErr != nil {
		h.logger.Error("compute next turn number", "error", turnErr, "conversation_id", convID)
		h.writeJSONMessage(ctx, conn, "error", map[string]string{"error": "failed to compute next turn number"})
		return
	}
	modelContextLimit, limitErr := h.resolveModelContextLimit(prov)
	if limitErr != nil {
		h.logger.Error("resolve model context limit", "error", limitErr, "provider", prov, "conversation_id", convID)
		h.writeJSONMessage(ctx, conn, "error", map[string]string{"error": "failed to resolve model context limit"})
		return
	}

	req := agent.RunTurnRequest{
		ConversationID:    convID,
		TurnNumber:        turnNumber,
		Message:           msg.Content,
		ModelContextLimit: modelContextLimit,
		Model:             model,
		Provider:          prov,
	}

	_, err := h.agent.RunTurn(ctx, req)
	if err != nil {
		h.logger.Error("run turn", "error", err, "conversation_id", convID)
		// ErrorEvent is emitted by the agent loop itself; no need to send another.
	}
}

// writeJSONMessage sends a JSON message to the WebSocket client.
func (h *WebSocketHandler) writeJSONMessage(ctx context.Context, conn *websocket.Conn, msgType string, data any) {
	msg := ServerMessage{
		Type:      msgType,
		Timestamp: time.Now(),
		Data:      data,
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		h.logger.Error("marshal websocket message", "error", err, "type", msgType)
		return
	}
	writeCtx, writeCancel := context.WithTimeout(ctx, 5*time.Second)
	defer writeCancel()
	if err := conn.Write(writeCtx, websocket.MessageText, raw); err != nil {
		h.logger.Debug("websocket writeJSONMessage failed", "error", err, "type", msgType)
	}
}
