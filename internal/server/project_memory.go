package server

import (
	"log/slog"
	"net/http"
)

type projectMemoryContractExporter interface {
	ExportContractJSON() ([]byte, error)
}

type projectMemoryProtocolHandler interface {
	HTTPHandler() http.Handler
	ProtocolEnabled() bool
}

type projectMemoryHandler struct {
	contract projectMemoryContractExporter
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
	s.HandleFunc("GET /api/project-memory/contract", h.handleContract)

	protocol, ok := backend.(projectMemoryProtocolHandler)
	if !ok || !protocol.ProtocolEnabled() {
		return
	}
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
