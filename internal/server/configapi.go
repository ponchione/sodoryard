package server

import (
	"context"
	"log/slog"
	"net/http"
	"sync"

	"github.com/ponchione/sirtopham/internal/config"
	"github.com/ponchione/sirtopham/internal/provider"
)

// ModelLister is the interface the config handler needs from the provider router.
type ModelLister interface {
	Models(ctx context.Context) ([]provider.Model, error)
}

// ConfigHandler serves config and provider endpoints.
type ConfigHandler struct {
	cfg        *config.Config
	providers  map[string]config.ProviderConfig
	models     ModelLister
	logger     *slog.Logger

	// Runtime overrides (not persisted to config file).
	mu                sync.RWMutex
	overrideProvider  string
	overrideModel     string
}

// NewConfigHandler creates a handler and registers routes on the server.
// modelLister can be nil if the provider router is not available.
func NewConfigHandler(s *Server, cfg *config.Config, modelLister ModelLister, logger *slog.Logger) *ConfigHandler {
	h := &ConfigHandler{
		cfg:       cfg,
		providers: cfg.Providers,
		models:    modelLister,
		logger:    logger,
	}

	s.HandleFunc("GET /api/config", h.handleGetConfig)
	s.HandleFunc("PUT /api/config", h.handlePutConfig)
	s.HandleFunc("GET /api/providers", h.handleProviders)

	return h
}

// ── GET /api/config ──────────────────────────────────────────────────

type configResponse struct {
	DefaultProvider  string         `json:"default_provider"`
	DefaultModel     string         `json:"default_model"`
	FallbackProvider string         `json:"fallback_provider"`
	FallbackModel    string         `json:"fallback_model"`
	Agent            agentSettings  `json:"agent"`
	Providers        []providerInfo `json:"providers"`
}

type agentSettings struct {
	MaxIterations    int  `json:"max_iterations"`
	ExtendedThinking bool `json:"extended_thinking"`
}

type providerInfo struct {
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	Models []string `json:"models,omitempty"`
}

func (h *ConfigHandler) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	overrideP := h.overrideProvider
	overrideM := h.overrideModel
	h.mu.RUnlock()

	defaultProvider := h.cfg.Routing.Default.Provider
	defaultModel := h.cfg.Routing.Default.Model
	if overrideP != "" {
		defaultProvider = overrideP
	}
	if overrideM != "" {
		defaultModel = overrideM
	}

	// Build provider list.
	var providers []providerInfo
	if h.models != nil {
		models, err := h.models.Models(r.Context())
		if err == nil {
			providers = h.buildProviderList(models)
		}
	}
	if providers == nil {
		// Fallback: list from config without model enumeration.
		for name, pc := range h.providers {
			pi := providerInfo{Name: name, Type: pc.Type}
			if pc.Model != "" {
				pi.Models = []string{pc.Model}
			}
			providers = append(providers, pi)
		}
	}

	writeJSON(w, http.StatusOK, configResponse{
		DefaultProvider:  defaultProvider,
		DefaultModel:     defaultModel,
		FallbackProvider: h.cfg.Routing.Fallback.Provider,
		FallbackModel:    h.cfg.Routing.Fallback.Model,
		Agent: agentSettings{
			MaxIterations:    h.cfg.Agent.MaxIterationsPerTurn,
			ExtendedThinking: h.cfg.Agent.ExtendedThinking,
		},
		Providers: providers,
	})
}

func (h *ConfigHandler) buildProviderList(models []provider.Model) []providerInfo {
	provModels := map[string][]string{}
	for name := range h.providers {
		provModels[name] = []string{}
	}
	for _, m := range models {
		// Models don't inherently carry a provider name, so list them globally
		// under all providers as available models.
		for name := range h.providers {
			provModels[name] = append(provModels[name], m.ID)
		}
	}

	var result []providerInfo
	for name, pc := range h.providers {
		result = append(result, providerInfo{
			Name:   name,
			Type:   pc.Type,
			Models: provModels[name],
		})
	}
	return result
}

// ── PUT /api/config ──────────────────────────────────────────────────

type updateConfigRequest struct {
	DefaultProvider *string `json:"default_provider,omitempty"`
	DefaultModel    *string `json:"default_model,omitempty"`
}

func (h *ConfigHandler) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	var req updateConfigRequest
	if !decodeJSON(w, r, &req, h.logger) {
		return
	}

	h.mu.Lock()
	if req.DefaultProvider != nil {
		// Validate provider name exists.
		if _, ok := h.providers[*req.DefaultProvider]; !ok {
			h.mu.Unlock()
			writeError(w, http.StatusBadRequest, "unknown provider: "+*req.DefaultProvider)
			return
		}
		h.overrideProvider = *req.DefaultProvider
	}
	if req.DefaultModel != nil {
		h.overrideModel = *req.DefaultModel
	}
	h.mu.Unlock()

	// Re-serve the updated config.
	h.handleGetConfig(w, r)
}

// ── GET /api/providers ───────────────────────────────────────────────

type providerStatus struct {
	Name   string          `json:"name"`
	Type   string          `json:"type"`
	Status string          `json:"status"` // "available" or "unavailable"
	Models []provider.Model `json:"models"`
}

func (h *ConfigHandler) handleProviders(w http.ResponseWriter, r *http.Request) {
	var result []providerStatus

	for name, pc := range h.providers {
		ps := providerStatus{
			Name:   name,
			Type:   pc.Type,
			Status: "available", // Default — we don't have health checks wired yet.
			Models: []provider.Model{},
		}

		// Try to get models from the router.
		if h.models != nil {
			models, err := h.models.Models(r.Context())
			if err != nil {
				ps.Status = "unavailable"
			} else {
				ps.Models = models
			}
		}

		result = append(result, ps)
	}

	writeJSON(w, http.StatusOK, result)
}
