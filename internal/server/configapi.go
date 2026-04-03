package server

import (
	"context"
	"log/slog"
	"net/http"
	"sync"

	"github.com/ponchione/sirtopham/internal/config"
	"github.com/ponchione/sirtopham/internal/provider"
	routerpkg "github.com/ponchione/sirtopham/internal/provider/router"
)

// ProviderRuntimeInspector is the interface the config handler needs from the provider router.
type ProviderRuntimeInspector interface {
	Models(ctx context.Context) ([]provider.Model, error)
	AuthStatuses(ctx context.Context) (map[string]*provider.AuthStatus, error)
	ProviderHealthMap() map[string]*routerpkg.ProviderHealth
}

// ConfigHandler serves config and provider endpoints.
type ConfigHandler struct {
	cfg       *config.Config
	providers map[string]config.ProviderConfig
	runtime   ProviderRuntimeInspector
	logger    *slog.Logger

	// Runtime overrides (not persisted to config file).
	mu               sync.RWMutex
	overrideProvider string
	overrideModel    string
}

// NewConfigHandler creates a handler and registers routes on the server.
// runtime can be nil if the provider router is not available.
func NewConfigHandler(s *Server, cfg *config.Config, runtime ProviderRuntimeInspector, logger *slog.Logger) *ConfigHandler {
	h := &ConfigHandler{
		cfg:       cfg,
		providers: cfg.Providers,
		runtime:   runtime,
		logger:    logger,
	}

	s.HandleFunc("GET /api/config", h.handleGetConfig)
	s.HandleFunc("PUT /api/config", h.handlePutConfig)
	s.HandleFunc("GET /api/providers", h.handleProviders)
	s.HandleFunc("GET /api/auth/providers", h.handleAuthProviders)

	return h
}

// ── GET /api/config ──────────────────────────────────────────────────

type configResponse struct {
	DefaultProvider string         `json:"default_provider"`
	DefaultModel    string         `json:"default_model"`
	Agent           agentSettings  `json:"agent"`
	Providers       []providerInfo `json:"providers"`
}

type agentSettings struct {
	MaxIterations       int    `json:"max_iterations"`
	ExtendedThinking    bool   `json:"extended_thinking"`
	ToolOutputMaxTokens int    `json:"tool_output_max_tokens"`
	ToolResultStoreRoot string `json:"tool_result_store_root"`
}

type providerInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`

	Models    []string             `json:"models,omitempty"`
	Status    string               `json:"status,omitempty"`
	Healthy   bool                 `json:"healthy"`
	LastError string               `json:"last_error,omitempty"`
	Auth      *provider.AuthStatus `json:"auth,omitempty"`
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

	var providers []providerInfo
	if h.runtime != nil {
		models, modelErr := h.runtime.Models(r.Context())
		authStatuses, authErr := h.runtime.AuthStatuses(r.Context())
		health := h.runtime.ProviderHealthMap()
		if modelErr == nil || authErr == nil {
			if modelErr != nil {
				models = nil
			}
			if authErr != nil {
				authStatuses = nil
			}
			providers = h.buildProviderList(models, authStatuses, health)
		}
	}
	if providers == nil {
		for name, pc := range h.providers {
			pi := providerInfo{Name: name, Type: pc.Type, Healthy: true, Status: "available"}
			if pc.Model != "" {
				pi.Models = []string{pc.Model}
			}
			providers = append(providers, pi)
		}
	}

	writeJSON(w, http.StatusOK, configResponse{
		DefaultProvider: defaultProvider,
		DefaultModel:    defaultModel,
		Agent: agentSettings{
			MaxIterations:       h.cfg.Agent.MaxIterationsPerTurn,
			ExtendedThinking:    h.cfg.Agent.ExtendedThinking,
			ToolOutputMaxTokens: h.cfg.Agent.ToolOutputMaxTokens,
			ToolResultStoreRoot: h.cfg.Agent.ToolResultStoreRoot,
		},
		Providers: providers,
	})
}

func (h *ConfigHandler) buildProviderList(models []provider.Model, authStatuses map[string]*provider.AuthStatus, health map[string]*routerpkg.ProviderHealth) []providerInfo {
	provModels := map[string][]string{}
	for name := range h.providers {
		provModels[name] = []string{}
	}
	for _, m := range models {
		name := m.Provider
		if name == "" {
			continue
		}
		if _, ok := provModels[name]; ok {
			provModels[name] = append(provModels[name], m.ID)
		}
	}

	var result []providerInfo
	for name, pc := range h.providers {
		lastError := ""
		healthy := false
		status := "unavailable"
		if hp, ok := health[name]; ok && hp != nil {
			healthy = hp.Healthy
			if healthy {
				status = "available"
			} else if hp.LastError != nil {
				lastError = hp.LastError.Error()
			}
		}
		result = append(result, providerInfo{
			Name:      name,
			Type:      pc.Type,
			Models:    provModels[name],
			Status:    status,
			Healthy:   healthy,
			LastError: lastError,
			Auth:      authStatuses[name],
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

	h.handleGetConfig(w, r)
}

// ── GET /api/providers ───────────────────────────────────────────────

type providerStatus struct {
	Name      string               `json:"name"`
	Type      string               `json:"type"`
	Status    string               `json:"status"`
	Healthy   bool                 `json:"healthy"`
	LastError string               `json:"last_error,omitempty"`
	Models    []provider.Model     `json:"models"`
	Auth      *provider.AuthStatus `json:"auth,omitempty"`
}

func (h *ConfigHandler) handleProviders(w http.ResponseWriter, r *http.Request) {
	provModels := map[string][]provider.Model{}
	authStatuses := map[string]*provider.AuthStatus{}
	health := map[string]*routerpkg.ProviderHealth{}
	if h.runtime != nil {
		all, err := h.runtime.Models(r.Context())
		if err == nil {
			for _, m := range all {
				if m.Provider != "" {
					provModels[m.Provider] = append(provModels[m.Provider], m)
				}
			}
		}
		if statuses, err := h.runtime.AuthStatuses(r.Context()); err == nil {
			authStatuses = statuses
		}
		health = h.runtime.ProviderHealthMap()
	}

	var result []providerStatus
	for name, pc := range h.providers {
		status := "unavailable"
		healthy := false
		lastError := ""
		if hp, ok := health[name]; ok && hp != nil {
			healthy = hp.Healthy
			if healthy {
				status = "available"
			} else if hp.LastError != nil {
				lastError = hp.LastError.Error()
			}
		}
		ps := providerStatus{
			Name:      name,
			Type:      pc.Type,
			Status:    status,
			Healthy:   healthy,
			LastError: lastError,
			Models:    provModels[name],
			Auth:      authStatuses[name],
		}
		if ps.Models == nil {
			ps.Models = []provider.Model{}
		}
		result = append(result, ps)
	}

	writeJSON(w, http.StatusOK, result)
}

type authProviderStatus struct {
	Name      string               `json:"name"`
	Type      string               `json:"type"`
	Status    string               `json:"status"`
	Healthy   bool                 `json:"healthy"`
	LastError string               `json:"last_error,omitempty"`
	Auth      *provider.AuthStatus `json:"auth,omitempty"`
}

func (h *ConfigHandler) handleAuthProviders(w http.ResponseWriter, r *http.Request) {
	authStatuses := map[string]*provider.AuthStatus{}
	health := map[string]*routerpkg.ProviderHealth{}
	if h.runtime != nil {
		if statuses, err := h.runtime.AuthStatuses(r.Context()); err == nil {
			authStatuses = statuses
		}
		health = h.runtime.ProviderHealthMap()
	}

	var result []authProviderStatus
	for name, pc := range h.providers {
		status := "unavailable"
		healthy := false
		lastError := ""
		if hp, ok := health[name]; ok && hp != nil {
			healthy = hp.Healthy
			if healthy {
				status = "available"
			} else if hp.LastError != nil {
				lastError = hp.LastError.Error()
			}
		}
		result = append(result, authProviderStatus{
			Name:      name,
			Type:      pc.Type,
			Status:    status,
			Healthy:   healthy,
			LastError: lastError,
			Auth:      authStatuses[name],
		})
	}
	writeJSON(w, http.StatusOK, result)
}
