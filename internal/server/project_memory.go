package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/ponchione/sodoryard/internal/projectmemory"
)

type projectMemoryContractExporter interface {
	ExportContractJSON() ([]byte, error)
}

type projectMemoryProtocolHandler interface {
	HTTPHandler() http.Handler
	ProtocolEnabled() bool
}

type projectMemoryProtocolTokenMinter interface {
	MintProtocolToken() (projectmemory.ProtocolToken, error)
}

type projectMemoryHandler struct {
	contract projectMemoryContractExporter
	protocol projectMemoryProtocolHandler
	tokens   projectMemoryProtocolTokenMinter
	logger   *slog.Logger
}

func NewProjectMemoryHandler(s *Server, backend any, logger *slog.Logger) {
	if s == nil {
		return
	}
	h := &projectMemoryHandler{logger: logger}
	if exporter, ok := backend.(projectMemoryContractExporter); ok {
		h.contract = exporter
	}
	if minter, ok := backend.(projectMemoryProtocolTokenMinter); ok {
		h.tokens = minter
	}
	s.HandleFunc("GET /api/project-memory/contract", h.handleContract)
	s.HandleFunc("POST /api/project-memory/token", h.handleToken)

	protocol, ok := backend.(projectMemoryProtocolHandler)
	if !ok || !protocol.ProtocolEnabled() {
		return
	}
	h.protocol = protocol
	s.Handle("/api/project-memory/", http.StripPrefix("/api/project-memory", protocol.HTTPHandler()))
}

func (h *projectMemoryHandler) handleContract(w http.ResponseWriter, r *http.Request) {
	if h.contract == nil {
		http.Error(w, `{"error":"project memory contract unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	data, err := h.contract.ExportContractJSON()
	if err != nil {
		if h.logger != nil {
			h.logger.Error("export project memory contract", "error", err)
		}
		http.Error(w, `{"error":"project memory contract unavailable"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

type projectMemoryTokenResponse struct {
	Token     string `json:"token"`
	TokenType string `json:"token_type"`
	Identity  string `json:"identity,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

func (h *projectMemoryHandler) handleToken(w http.ResponseWriter, r *http.Request) {
	if h.tokens == nil || h.protocol == nil || !h.protocol.ProtocolEnabled() {
		writeError(w, http.StatusServiceUnavailable, "project memory protocol token minting is unavailable")
		return
	}
	token, err := h.tokens.MintProtocolToken()
	if err != nil {
		if h.logger != nil {
			h.logger.Error("mint project memory protocol token", "error", err)
		}
		writeError(w, http.StatusInternalServerError, "project memory protocol token unavailable")
		return
	}
	resp := projectMemoryTokenResponse{
		Token:     token.Token,
		TokenType: "bearer",
		Identity:  token.Identity,
	}
	if !token.ExpiresAt.IsZero() {
		resp.ExpiresAt = token.ExpiresAt.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, resp)
}
