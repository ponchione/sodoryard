package server

import (
	"context"
	"encoding/json"
	"errors"
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

// WebSocketHandler handles WebSocket connections for streaming agent events.
type WebSocketHandler struct {
	agent     AgentService
	convSvc   ConversationService
	cfg       *config.Config
	projectID string
	defaults  *RuntimeDefaults
	devMode   bool
	logger    *slog.Logger

	activeTurn atomic.Bool
}

var devWebSocketOriginPatterns = []string{
	"localhost:5173",
	"127.0.0.1:5173",
	"[::1]:5173",
}

// NewWebSocketHandler creates a handler and registers the WS route.
func NewWebSocketHandler(s *Server, agentSvc AgentService, convSvc ConversationService, cfg *config.Config, defaults *RuntimeDefaults, logger *slog.Logger) *WebSocketHandler {
	if defaults == nil {
		defaults = NewRuntimeDefaults(cfg)
	}
	h := &WebSocketHandler{
		agent:     agentSvc,
		convSvc:   convSvc,
		cfg:       cfg,
		projectID: cfg.ProjectRoot,
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

func (h *WebSocketHandler) handleWS(w http.ResponseWriter, r *http.Request) {
	acceptOptions := &websocket.AcceptOptions{}
	if h.devMode {
		// In dev mode, Vite dev server connects from a different origin.
		acceptOptions.OriginPatterns = devWebSocketOriginPatterns
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

	// Read loop goroutine (client → server).
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		h.readLoop(ctx, cancel, conn, sink, &turnActive)
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
func (h *WebSocketHandler) readLoop(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, sink *agent.ChannelSink, turnActive *atomic.Bool) {
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
			h.writeErrorMessage(ctx, conn, "invalid message", false, "invalid_message")
			continue
		}

		switch msg.Type {
		case "message":
			if !turnActive.CompareAndSwap(false, true) {
				h.writeErrorMessage(ctx, conn, "a turn is already in progress", true, "turn_in_progress")
				continue
			}
			if !h.activeTurn.CompareAndSwap(false, true) {
				turnActive.Store(false)
				h.writeErrorMessage(ctx, conn, "a turn is already in progress", true, "turn_in_progress")
				continue
			}

			turnWg.Add(1)
			go func() {
				defer turnWg.Done()
				defer h.activeTurn.Store(false)
				defer turnActive.Store(false)
				h.handleMessage(ctx, conn, sink, msg)
			}()

		case "model_override":
			// Runtime is intentionally pinned for now; ignore per-connection overrides.
			h.logger.Info("model override ignored; runtime locked",
				"model", msg.Model,
				"provider", msg.Provider,
			)

		case "cancel":
			if turnActive.Load() {
				h.agent.Cancel()
				continue
			}
			h.writeErrorMessage(ctx, conn, "no active turn to cancel", true, "no_active_turn")

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
	return h.convSvc.NextTurnNumber(ctx, conversationID)
}

func (h *WebSocketHandler) handleMessage(ctx context.Context, conn *websocket.Conn, sink *agent.ChannelSink, msg ClientMessage) {
	// Subscribe sink to receive events for this turn.
	h.agent.Subscribe(sink)
	defer h.agent.Unsubscribe(sink)

	convID := msg.ConversationID

	var conversationDefaults *conversation.Conversation
	if convID != "" {
		stored, err := h.convSvc.Get(ctx, convID)
		if err != nil {
			h.logger.Error("load conversation defaults", "error", err, "conversation_id", convID)
			h.writeErrorMessage(ctx, conn, "failed to load conversation", false, "conversation_load_failed")
			return
		}
		conversationDefaults = stored
	}

	prov, model := lockedRuntimeDefault()
	if h.defaults != nil {
		if defaultProvider, defaultModel := h.defaults.Get(); defaultProvider != "" && defaultModel != "" {
			prov, model = defaultProvider, defaultModel
		}
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
			h.writeErrorMessage(ctx, conn, "failed to create conversation", false, "conversation_create_failed")
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
				h.writeErrorMessage(ctx, conn, "failed to persist conversation defaults", false, "conversation_defaults_persist_failed")
				return
			}
		}
	}

	turnNumber, turnErr := h.nextTurnNumber(ctx, convID)
	if turnErr != nil {
		h.logger.Error("compute next turn number", "error", turnErr, "conversation_id", convID)
		h.writeErrorMessage(ctx, conn, "failed to compute next turn number", false, "turn_number_failed")
		return
	}
	modelContextLimit, limitErr := config.ResolveModelContextLimit(h.cfg, prov)
	if limitErr != nil {
		h.logger.Error("resolve model context limit", "error", limitErr, "provider", prov, "conversation_id", convID)
		h.writeErrorMessage(ctx, conn, "failed to resolve model context limit", false, "model_context_limit_failed")
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
		if errors.Is(err, agent.ErrTurnCancelled) {
			h.logger.Info("run turn cancelled", "error", err, "conversation_id", convID)
			return
		}
		h.logger.Error("run turn", "error", err, "conversation_id", convID)
		// ErrorEvent is emitted by the agent loop itself; no need to send another.
	}
}

type wsErrorData struct {
	Message     string `json:"message"`
	Recoverable bool   `json:"recoverable,omitempty"`
	ErrorCode   string `json:"error_code,omitempty"`
}

func (h *WebSocketHandler) writeErrorMessage(ctx context.Context, conn *websocket.Conn, message string, recoverable bool, errorCode string) {
	h.writeJSONMessage(ctx, conn, "error", wsErrorData{Message: message, Recoverable: recoverable, ErrorCode: errorCode})
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
		h.logger.Debug("websocket write failed", "error", err, "type", msgType)
	}
}
