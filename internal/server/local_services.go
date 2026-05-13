package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/ponchione/sodoryard/internal/localservices"
)

type localServicesCommandResponse struct {
	Status  localservices.StackStatus `json:"status"`
	Message string                    `json:"message"`
	Error   string                    `json:"error,omitempty"`
}

type localServicesLogsResponse struct {
	Tail int    `json:"tail"`
	Logs string `json:"logs"`
}

func (h *ChainInspectorHandler) handleLocalServicesStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.svc.LocalServicesStatus(r.Context())
	if err != nil {
		h.logger.Warn("local services status", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *ChainInspectorHandler) handleLocalServicesUp(w http.ResponseWriter, r *http.Request) {
	status, err := h.svc.LocalServicesUp(r.Context())
	if err != nil {
		h.logger.Warn("local services up", "error", err)
		writeJSON(w, http.StatusConflict, localServicesCommandResponse{
			Status:  status,
			Message: "local services not ready",
			Error:   err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, localServicesCommandResponse{Status: status, Message: "local services ready"})
}

func (h *ChainInspectorHandler) handleLocalServicesDown(w http.ResponseWriter, r *http.Request) {
	status, err := h.svc.LocalServicesDown(r.Context())
	if err != nil {
		h.logger.Warn("local services down", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, localServicesCommandResponse{Status: status, Message: "local services stopped"})
}

func (h *ChainInspectorHandler) handleLocalServicesLogs(w http.ResponseWriter, r *http.Request) {
	tail, err := parseLogTail(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	logs, err := h.svc.LocalServicesLogs(r.Context(), tail)
	if err != nil {
		h.logger.Warn("local services logs", "tail", tail, "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, localServicesLogsResponse{Tail: tail, Logs: logs})
}

func parseLogTail(r *http.Request) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("tail"))
	if raw == "" {
		return 200, nil
	}
	tail, err := strconv.Atoi(raw)
	if err != nil || tail < 0 {
		return 0, fmt.Errorf("tail must be a non-negative integer")
	}
	if tail > 5000 {
		tail = 5000
	}
	return tail, nil
}
