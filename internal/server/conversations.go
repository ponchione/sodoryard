package server

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/ponchione/sodoryard/internal/conversation"
)

const (
	defaultMessagePageLimit = 200
	maxMessagePageLimit     = 500
)

// ConversationService is the interface the conversation handlers need.
// Satisfied by *conversation.Manager.
type ConversationService interface {
	Create(ctx context.Context, projectID string, opts ...conversation.CreateOption) (*conversation.Conversation, error)
	Get(ctx context.Context, conversationID string) (*conversation.Conversation, error)
	List(ctx context.Context, projectID string, limit, offset int) ([]conversation.ConversationSummary, error)
	Delete(ctx context.Context, conversationID string) error
	SetRuntimeDefaults(ctx context.Context, conversationID string, provider, model *string) error
	NextTurnNumber(ctx context.Context, conversationID string) (int, error)
	GetMessages(ctx context.Context, conversationID string) ([]conversation.MessageView, error)
	GetMessagePage(ctx context.Context, conversationID string, limit, offset int) ([]conversation.MessageView, error)
	Search(ctx context.Context, query string) ([]conversation.SearchResult, error)
}

// ConversationHandler handles conversation REST endpoints.
type ConversationHandler struct {
	service   ConversationService
	projectID string
	logger    *slog.Logger
}

// NewConversationHandler creates a handler and registers routes on the server.
func NewConversationHandler(s *Server, svc ConversationService, projectID string, logger *slog.Logger) *ConversationHandler {
	h := &ConversationHandler{service: svc, projectID: projectID, logger: logger}

	// Register routes — search must come before {id} so the literal "search"
	// is matched before the wildcard. Go 1.22+ handles this correctly.
	s.HandleFunc("GET /api/conversations/search", h.handleSearch)
	s.HandleFunc("GET /api/conversations/{id}", h.handleGet)
	s.HandleFunc("GET /api/conversations/{id}/messages", h.handleMessages)
	s.HandleFunc("GET /api/conversations", h.handleList)
	s.HandleFunc("POST /api/conversations", h.handleCreate)
	s.HandleFunc("DELETE /api/conversations/{id}", h.handleDelete)

	return h
}

func (h *ConversationHandler) handleList(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}

	convos, err := h.service.List(r.Context(), h.projectID, limit, offset)
	if err != nil {
		h.logger.Error("list conversations", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list conversations")
		return
	}
	writeJSON(w, http.StatusOK, convos)
}

func (h *ConversationHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title    string `json:"title"`
		Model    string `json:"model"`
		Provider string `json:"provider"`
	}
	if r.ContentLength > 0 {
		if !decodeJSON(w, r, &req, h.logger) {
			return
		}
	}

	var opts []conversation.CreateOption
	if req.Title != "" {
		opts = append(opts, conversation.WithTitle(req.Title))
	}
	if req.Model != "" {
		opts = append(opts, conversation.WithModel(req.Model))
	}
	if req.Provider != "" {
		opts = append(opts, conversation.WithProvider(req.Provider))
	}

	c, err := h.service.Create(r.Context(), h.projectID, opts...)
	if err != nil {
		h.logger.Error("create conversation", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create conversation")
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (h *ConversationHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, err := h.service.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "conversation not found")
			return
		}
		h.logger.Error("get conversation", "error", err, "id", id)
		writeError(w, http.StatusInternalServerError, "failed to get conversation")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *ConversationHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.service.Delete(r.Context(), id); err != nil {
		h.logger.Error("delete conversation", "error", err, "id", id)
		writeError(w, http.StatusInternalServerError, "failed to delete conversation")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *ConversationHandler) handleMessages(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = defaultMessagePageLimit
	} else if limit > maxMessagePageLimit {
		limit = maxMessagePageLimit
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}

	msgs, err := h.service.GetMessagePage(r.Context(), id, limit, offset)
	if err != nil {
		h.logger.Error("get messages", "error", err, "id", id)
		writeError(w, http.StatusInternalServerError, "failed to get messages")
		return
	}
	writeJSON(w, http.StatusOK, msgs)
}

func (h *ConversationHandler) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "query parameter 'q' is required")
		return
	}
	results, err := h.service.Search(r.Context(), q)
	if err != nil {
		h.logger.Error("search conversations", "error", err, "query", q)
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}
	writeJSON(w, http.StatusOK, results)
}
