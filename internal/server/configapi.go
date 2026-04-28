package server

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/provider"
	routerpkg "github.com/ponchione/sodoryard/internal/provider/router"
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
	defaults  *RuntimeDefaults
	logger    *slog.Logger
}

// NewConfigHandler creates a handler and registers routes on the server.
// runtime can be nil if the provider router is not available.
func NewConfigHandler(s *Server, cfg *config.Config, runtime ProviderRuntimeInspector, defaults *RuntimeDefaults, logger *slog.Logger) *ConfigHandler {
	if defaults == nil {
		defaults = NewRuntimeDefaults(cfg)
	}
	h := &ConfigHandler{
		cfg:       cfg,
		providers: cfg.Providers,
		runtime:   runtime,
		defaults:  defaults,
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
	DefaultProvider  string         `json:"default_provider"`
	DefaultModel     string         `json:"default_model"`
	FallbackProvider string         `json:"fallback_provider"`
	FallbackModel    string         `json:"fallback_model"`
	Agent            agentSettings  `json:"agent"`
	Providers        []providerInfo `json:"providers"`
}

type agentSettings struct {
	MaxIterations            int    `json:"max_iterations"`
	ExtendedThinking         bool   `json:"extended_thinking"`
	ToolOutputMaxTokens      int    `json:"tool_output_max_tokens"`
	ToolResultStoreRoot      string `json:"tool_result_store_root"`
	CacheSystemPrompt        bool   `json:"cache_system_prompt"`
	CacheAssembledContext    bool   `json:"cache_assembled_context"`
	CacheConversationHistory bool   `json:"cache_conversation_history"`
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
	defaultProvider, defaultModel := h.defaults.Get()
	if defaultProvider == "" {
		defaultProvider = h.cfg.Routing.Default.Provider
	}
	if defaultModel == "" {
		defaultModel = h.cfg.Routing.Default.Model
	}

	providerNames := h.cfg.ProviderNamesForSurfaces()

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
			providers = h.buildProviderList(providerNames, models, authStatuses, health)
		}
	}
	if providers == nil {
		for _, name := range providerNames {
			pc, ok := h.providers[name]
			if !ok {
				continue
			}
			pi := providerInfo{Name: name, Type: pc.Type, Healthy: true, Status: "available"}
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
			MaxIterations:            h.cfg.Agent.MaxIterationsPerTurn,
			ExtendedThinking:         h.cfg.Agent.ExtendedThinking,
			ToolOutputMaxTokens:      h.cfg.Agent.ToolOutputMaxTokens,
			ToolResultStoreRoot:      h.cfg.Agent.ToolResultStoreRoot,
			CacheSystemPrompt:        h.cfg.Agent.CacheSystemPrompt,
			CacheAssembledContext:    h.cfg.Agent.CacheAssembledContext,
			CacheConversationHistory: h.cfg.Agent.CacheConversationHistory,
		},
		Providers: providers,
	})
}

func (h *ConfigHandler) availableModelsByProvider(ctx context.Context) map[string]map[string]struct{} {
	providerNames := h.cfg.ProviderNamesForSurfaces()
	available := make(map[string]map[string]struct{}, len(providerNames))
	for _, name := range providerNames {
		pc, ok := h.providers[name]
		if !ok {
			continue
		}
		models := map[string]struct{}{}
		if pc.Model != "" {
			models[pc.Model] = struct{}{}
		}
		available[name] = models
	}
	if h.runtime == nil {
		return available
	}
	models, err := h.runtime.Models(ctx)
	if err != nil {
		return available
	}
	for _, m := range models {
		if m.Provider == "" {
			continue
		}
		if _, ok := available[m.Provider]; !ok {
			continue
		}
		available[m.Provider][m.ID] = struct{}{}
	}
	return available
}

func (h *ConfigHandler) buildProviderList(providerNames []string, models []provider.Model, authStatuses map[string]*provider.AuthStatus, health map[string]*routerpkg.ProviderHealth) []providerInfo {
	provModels := map[string][]string{}
	for _, name := range providerNames {
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
	for _, name := range providerNames {
		pc, ok := h.providers[name]
		if !ok {
			continue
		}
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

	provider, model := h.defaults.Get()
	if req.DefaultProvider != nil {
		if _, ok := h.providers[*req.DefaultProvider]; !ok {
			writeError(w, http.StatusBadRequest, "unknown provider: "+*req.DefaultProvider)
			return
		}
		provider = *req.DefaultProvider
	}
	if req.DefaultModel != nil {
		model = *req.DefaultModel
	}
	availableModels := h.availableModelsByProvider(r.Context())
	providerModels, ok := availableModels[provider]
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown provider: "+provider)
		return
	}
	if model != "" && len(providerModels) > 0 {
		if _, ok := providerModels[model]; !ok {
			writeError(w, http.StatusBadRequest, "model "+model+" not available on provider "+provider)
			return
		}
	}
	if !runtimeDefaultOverrideAllowed(provider, model) {
		writeError(w, http.StatusBadRequest, "runtime default override is locked to codex/gpt-5.5")
		return
	}
	h.defaults.Set(provider, model)

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
	for _, name := range h.cfg.ProviderNamesForSurfaces() {
		pc, ok := h.providers[name]
		if !ok {
			continue
		}
		status, healthy, lastError := providerHealthSummary(health[name])
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
	for _, name := range h.cfg.ProviderNamesForSurfaces() {
		pc, ok := h.providers[name]
		if !ok {
			continue
		}
		status, healthy, lastError := providerHealthSummary(health[name])
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

func providerHealthSummary(hp *routerpkg.ProviderHealth) (string, bool, string) {
	if hp == nil {
		return "unavailable", false, ""
	}
	if hp.Healthy {
		return "available", true, ""
	}
	if hp.LastError != nil {
		return "unavailable", false, hp.LastError.Error()
	}
	return "unavailable", false, ""
}
