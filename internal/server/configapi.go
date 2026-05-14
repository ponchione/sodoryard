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

type ProviderCredentialRefresher interface {
	RefreshAuth(ctx context.Context, providerName string) (*provider.AuthStatus, error)
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
	s.HandleFunc("POST /api/auth/providers/{name}/refresh", h.handleRefreshProviderAuth)

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
		runtime := h.collectProviderRuntimeData(r.Context(), true, true)
		if runtime.modelsOK || runtime.authOK {
			providers = h.buildProviderList(providerNames, runtime)
		}
	}
	if providers == nil {
		providers = h.configuredProviderList(providerNames)
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

func (h *ConfigHandler) configuredProviderList(providerNames []string) []providerInfo {
	var result []providerInfo
	for _, name := range providerNames {
		pc, ok := h.providers[name]
		if !ok {
			continue
		}
		pi := providerInfo{Name: name, Type: pc.Type, Healthy: true, Status: "available"}
		if pc.Model != "" {
			pi.Models = []string{pc.Model}
		}
		result = append(result, pi)
	}
	return result
}

func (h *ConfigHandler) buildProviderList(providerNames []string, runtime providerRuntimeData) []providerInfo {
	var result []providerInfo
	for _, name := range providerNames {
		pc, ok := h.providers[name]
		if !ok {
			continue
		}
		status, healthy, lastError := providerHealthSummary(runtime.health[name])
		result = append(result, providerInfo{
			Name:      name,
			Type:      pc.Type,
			Models:    providerModelIDs(runtime.modelsByProvider[name]),
			Status:    status,
			Healthy:   healthy,
			LastError: lastError,
			Auth:      runtime.authStatuses[name],
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
	runtime := h.collectProviderRuntimeData(r.Context(), true, true)

	var result []providerStatus
	for _, name := range h.cfg.ProviderNamesForSurfaces() {
		pc, ok := h.providers[name]
		if !ok {
			continue
		}
		status, healthy, lastError := providerHealthSummary(runtime.health[name])
		models := runtime.modelsByProvider[name]
		if models == nil {
			models = []provider.Model{}
		}
		ps := providerStatus{
			Name:      name,
			Type:      pc.Type,
			Status:    status,
			Healthy:   healthy,
			LastError: lastError,
			Models:    models,
			Auth:      runtime.authStatuses[name],
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

type authProviderRefreshResponse struct {
	Name      string               `json:"name"`
	Type      string               `json:"type"`
	Status    string               `json:"status"`
	Healthy   bool                 `json:"healthy"`
	LastError string               `json:"last_error,omitempty"`
	Auth      *provider.AuthStatus `json:"auth,omitempty"`
	Message   string               `json:"message"`
}

func (h *ConfigHandler) handleAuthProviders(w http.ResponseWriter, r *http.Request) {
	runtime := h.collectProviderRuntimeData(r.Context(), false, true)

	var result []authProviderStatus
	for _, name := range h.cfg.ProviderNamesForSurfaces() {
		pc, ok := h.providers[name]
		if !ok {
			continue
		}
		status, healthy, lastError := providerHealthSummary(runtime.health[name])
		result = append(result, authProviderStatus{
			Name:      name,
			Type:      pc.Type,
			Status:    status,
			Healthy:   healthy,
			LastError: lastError,
			Auth:      runtime.authStatuses[name],
		})
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *ConfigHandler) handleRefreshProviderAuth(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	pc, ok := h.providers[name]
	if !ok {
		writeError(w, http.StatusNotFound, "unknown provider: "+name)
		return
	}
	if h.runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "provider credential refresh is unavailable")
		return
	}
	refresher, ok := h.runtime.(ProviderCredentialRefresher)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider credential refresh is not supported by this runtime")
		return
	}
	auth, err := refresher.RefreshAuth(r.Context(), name)
	if err != nil {
		h.logger.Warn("refresh provider credentials", "provider", name, "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	status, healthy, lastError := providerHealthSummary(h.runtime.ProviderHealthMap()[name])
	writeJSON(w, http.StatusOK, authProviderRefreshResponse{
		Name:      name,
		Type:      pc.Type,
		Status:    status,
		Healthy:   healthy,
		LastError: lastError,
		Auth:      auth,
		Message:   "provider credentials refreshed",
	})
}

type providerRuntimeData struct {
	modelsByProvider map[string][]provider.Model
	authStatuses     map[string]*provider.AuthStatus
	health           map[string]*routerpkg.ProviderHealth
	modelsOK         bool
	authOK           bool
}

func (h *ConfigHandler) collectProviderRuntimeData(ctx context.Context, includeModels bool, includeAuth bool) providerRuntimeData {
	data := providerRuntimeData{
		modelsByProvider: map[string][]provider.Model{},
		authStatuses:     map[string]*provider.AuthStatus{},
		health:           map[string]*routerpkg.ProviderHealth{},
	}
	if h.runtime == nil {
		return data
	}
	if includeModels {
		models, err := h.runtime.Models(ctx)
		if err == nil {
			data.modelsOK = true
			data.modelsByProvider = modelsByProvider(models)
		}
	}
	if includeAuth {
		statuses, err := h.runtime.AuthStatuses(ctx)
		if err == nil {
			data.authOK = true
			data.authStatuses = statuses
		}
	}
	data.health = h.runtime.ProviderHealthMap()
	return data
}

func modelsByProvider(models []provider.Model) map[string][]provider.Model {
	grouped := map[string][]provider.Model{}
	for _, m := range models {
		if m.Provider != "" {
			grouped[m.Provider] = append(grouped[m.Provider], m)
		}
	}
	return grouped
}

func providerModelIDs(models []provider.Model) []string {
	ids := make([]string, 0, len(models))
	for _, m := range models {
		ids = append(ids, m.ID)
	}
	return ids
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
