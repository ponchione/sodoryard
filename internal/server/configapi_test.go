package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/provider"
	router "github.com/ponchione/sodoryard/internal/provider/router"
	"github.com/ponchione/sodoryard/internal/server"
)

func TestGetConfigIncludesToolOutputLimitAndStoreRoot(t *testing.T) {
	projectRoot := t.TempDir()
	cfg := config.Default()
	cfg.ProjectRoot = projectRoot
	cfg.Brain.Enabled = false
	cfg.Agent.ToolResultStoreRoot = filepath.Join(projectRoot, ".artifacts", "tool-results")
	cfg.Agent.MaxIterationsPerTurn = 42
	cfg.Agent.ExtendedThinking = false
	cfg.Routing.Fallback.Provider = "openrouter"
	cfg.Routing.Fallback.Model = cfg.Providers["openrouter"].Model

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewConfigHandler(srv, cfg, nil, nil, newTestLogger())
	_, base := startServer(t, srv)

	resp, err := http.Get(base + "/api/config")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var body struct {
		DefaultProvider  string `json:"default_provider"`
		DefaultModel     string `json:"default_model"`
		FallbackProvider string `json:"fallback_provider"`
		FallbackModel    string `json:"fallback_model"`
		Agent            struct {
			MaxIterations            int    `json:"max_iterations"`
			ExtendedThinking         bool   `json:"extended_thinking"`
			ToolOutputMaxTokens      int    `json:"tool_output_max_tokens"`
			ToolResultStoreRoot      string `json:"tool_result_store_root"`
			CacheSystemPrompt        bool   `json:"cache_system_prompt"`
			CacheAssembledContext    bool   `json:"cache_assembled_context"`
			CacheConversationHistory bool   `json:"cache_conversation_history"`
		} `json:"agent"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body.DefaultProvider != "codex" {
		t.Fatalf("default_provider = %q, want %q", body.DefaultProvider, "codex")
	}
	if body.DefaultModel != "gpt-5.5" {
		t.Fatalf("default_model = %q, want %q", body.DefaultModel, "gpt-5.5")
	}
	if body.FallbackProvider != cfg.Routing.Fallback.Provider {
		t.Fatalf("fallback_provider = %q, want %q", body.FallbackProvider, cfg.Routing.Fallback.Provider)
	}
	if body.FallbackModel != cfg.Routing.Fallback.Model {
		t.Fatalf("fallback_model = %q, want %q", body.FallbackModel, cfg.Routing.Fallback.Model)
	}
	if body.Agent.MaxIterations != 42 {
		t.Fatalf("agent.max_iterations = %d, want 42", body.Agent.MaxIterations)
	}
	if body.Agent.ExtendedThinking {
		t.Fatal("agent.extended_thinking = true, want false")
	}
	if body.Agent.ToolOutputMaxTokens != cfg.Agent.ToolOutputMaxTokens {
		t.Fatalf("agent.tool_output_max_tokens = %d, want %d", body.Agent.ToolOutputMaxTokens, cfg.Agent.ToolOutputMaxTokens)
	}
	if body.Agent.ToolResultStoreRoot != cfg.Agent.ToolResultStoreRoot {
		t.Fatalf("agent.tool_result_store_root = %q, want %q", body.Agent.ToolResultStoreRoot, cfg.Agent.ToolResultStoreRoot)
	}
	if body.Agent.CacheSystemPrompt != cfg.Agent.CacheSystemPrompt {
		t.Fatalf("agent.cache_system_prompt = %t, want %t", body.Agent.CacheSystemPrompt, cfg.Agent.CacheSystemPrompt)
	}
	if body.Agent.CacheAssembledContext != cfg.Agent.CacheAssembledContext {
		t.Fatalf("agent.cache_assembled_context = %t, want %t", body.Agent.CacheAssembledContext, cfg.Agent.CacheAssembledContext)
	}
	if body.Agent.CacheConversationHistory != cfg.Agent.CacheConversationHistory {
		t.Fatalf("agent.cache_conversation_history = %t, want %t", body.Agent.CacheConversationHistory, cfg.Agent.CacheConversationHistory)
	}
}

// stubRuntimeInspector returns fixed model/auth/health data for testing.
type stubRuntimeInspector struct {
	models       []provider.Model
	modelsErr    error
	authStatuses map[string]*provider.AuthStatus
	authErr      error
	health       map[string]*router.ProviderHealth
}

func (s *stubRuntimeInspector) Models(_ context.Context) ([]provider.Model, error) {
	return s.models, s.modelsErr
}

func (s *stubRuntimeInspector) AuthStatuses(_ context.Context) (map[string]*provider.AuthStatus, error) {
	return s.authStatuses, s.authErr
}

func (s *stubRuntimeInspector) ProviderHealthMap() map[string]*router.ProviderHealth {
	return s.health
}

func TestPutConfigRejectsRuntimeDefaultOverrideAwayFromForcedCodexGPT55(t *testing.T) {
	cfg := &config.Config{
		ProjectRoot: t.TempDir(),
		Routing:     config.RoutingConfig{Default: config.RouteConfig{Provider: "codex", Model: "gpt-5.5"}},
		Providers: map[string]config.ProviderConfig{
			"codex": {Type: "codex", Model: "gpt-5.5"},
			"local": {Type: "openai-compatible", Model: "other-model"},
		},
	}
	runtime := &stubRuntimeInspector{models: []provider.Model{{ID: "gpt-5.5", Provider: "codex"}, {ID: "other-model", Provider: "local"}}}
	defaults := server.NewRuntimeDefaults(cfg)

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewConfigHandler(srv, cfg, runtime, defaults, newTestLogger())
	_, base := startServer(t, srv)

	body := []byte(`{"default_provider":"local","default_model":"other-model"}`)
	req, err := http.NewRequest(http.MethodPut, base+"/api/config", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /api/config failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	var errBody struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&errBody); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if !strings.Contains(errBody.Error, "locked") {
		t.Fatalf("error = %q, want locked message", errBody.Error)
	}
	provider, model := defaults.Get()
	if provider != "codex" || model != "gpt-5.5" {
		t.Fatalf("runtime defaults = (%q, %q), want (%q, %q)", provider, model, "codex", "gpt-5.5")
	}
}

func TestProvidersEndpointGroupsModelsByProvider(t *testing.T) {
	cfg := config.Default()
	cfg.ProjectRoot = t.TempDir()
	cfg.Brain.Enabled = false
	cfg.Providers = map[string]config.ProviderConfig{
		"anthropic": {Type: "anthropic", Model: "claude-sonnet-4-20250514"},
		"openai":    {Type: "openai", Model: "gpt-4o"},
	}

	runtime := &stubRuntimeInspector{models: []provider.Model{
		{ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4", Provider: "anthropic", ContextWindow: 200000, SupportsTools: true},
		{ID: "gpt-4o", Name: "GPT-4o", Provider: "openai", ContextWindow: 128000, SupportsTools: true},
	}, authStatuses: map[string]*provider.AuthStatus{
		"anthropic": {Provider: "anthropic", Mode: "api_key", Source: "env:ANTHROPIC_API_KEY", HasAccessToken: true},
		"openai":    {Provider: "openai", Detail: "auth status unavailable"},
	}, health: map[string]*router.ProviderHealth{
		"anthropic": {Healthy: true},
		"openai":    {Healthy: true},
	}}

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewConfigHandler(srv, cfg, runtime, nil, newTestLogger())
	_, base := startServer(t, srv)

	resp, err := http.Get(base + "/api/providers")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var result []struct {
		Name   string           `json:"name"`
		Models []provider.Model `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Sort for deterministic ordering.
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })

	if len(result) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(result))
	}

	// Anthropic should have only Claude models.
	if result[0].Name != "anthropic" {
		t.Fatalf("expected first provider = anthropic, got %q", result[0].Name)
	}
	if len(result[0].Models) != 1 || result[0].Models[0].ID != "claude-sonnet-4-20250514" {
		t.Fatalf("anthropic models: %+v", result[0].Models)
	}

	// OpenAI should have only GPT models.
	if result[1].Name != "openai" {
		t.Fatalf("expected second provider = openai, got %q", result[1].Name)
	}
	if len(result[1].Models) != 1 || result[1].Models[0].ID != "gpt-4o" {
		t.Fatalf("openai models: %+v", result[1].Models)
	}
}

func TestProvidersEndpointIncludesAuthAndHealth(t *testing.T) {
	cfg := config.Default()
	cfg.ProjectRoot = t.TempDir()
	cfg.Brain.Enabled = false
	cfg.Providers = map[string]config.ProviderConfig{
		"anthropic": {Type: "anthropic", Model: "claude-sonnet-4-20250514"},
		"openai":    {Type: "openai", Model: "gpt-4o"},
	}

	runtime := &stubRuntimeInspector{authStatuses: map[string]*provider.AuthStatus{
		"anthropic": {Provider: "anthropic", Mode: "api_key", Source: "env:ANTHROPIC_API_KEY", HasAccessToken: true},
	}, health: map[string]*router.ProviderHealth{
		"anthropic": {Healthy: true},
		"openai":    {Healthy: false, LastError: context.DeadlineExceeded},
	}}

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewConfigHandler(srv, cfg, runtime, nil, newTestLogger())
	_, base := startServer(t, srv)

	resp, err := http.Get(base + "/api/auth/providers")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var body []struct {
		Name      string               `json:"name"`
		Healthy   bool                 `json:"healthy"`
		LastError string               `json:"last_error"`
		Auth      *provider.AuthStatus `json:"auth"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	sort.Slice(body, func(i, j int) bool { return body[i].Name < body[j].Name })
	if len(body) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(body))
	}
	if body[0].Name != "anthropic" || body[0].Auth == nil || body[0].Auth.Mode != "api_key" {
		t.Fatalf("unexpected anthropic auth payload: %+v", body[0])
	}
	if body[1].Name != "openai" || body[1].Healthy || body[1].LastError == "" {
		t.Fatalf("expected unhealthy openai payload, got %+v", body[1])
	}
}

func TestAuthProvidersEndpointMarksMissingRuntimeProviderUnavailable(t *testing.T) {
	cfg := config.Default()
	cfg.ProjectRoot = t.TempDir()
	cfg.Brain.Enabled = false
	cfg.Providers = map[string]config.ProviderConfig{
		"anthropic": {Type: "anthropic", Model: "claude-sonnet-4-20250514"},
		"codex":     {Type: "codex", Model: "gpt-5.1-codex-mini"},
	}

	runtime := &stubRuntimeInspector{authStatuses: map[string]*provider.AuthStatus{
		"codex": {Provider: "codex", Mode: "oauth", Source: "sirtopham_store", HasAccessToken: true},
	}, health: map[string]*router.ProviderHealth{
		"codex": {Healthy: true},
	}}

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewConfigHandler(srv, cfg, runtime, nil, newTestLogger())
	_, base := startServer(t, srv)

	resp, err := http.Get(base + "/api/auth/providers")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var body []struct {
		Name    string `json:"name"`
		Status  string `json:"status"`
		Healthy bool   `json:"healthy"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	sort.Slice(body, func(i, j int) bool { return body[i].Name < body[j].Name })
	if len(body) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(body))
	}
	if body[0].Name != "anthropic" || body[0].Healthy || body[0].Status != "unavailable" {
		t.Fatalf("expected missing runtime provider to be unavailable, got %+v", body[0])
	}
	if body[1].Name != "codex" || !body[1].Healthy || body[1].Status != "available" {
		t.Fatalf("expected registered codex provider to stay available, got %+v", body[1])
	}
}

func TestConfigEndpointGroupsModelsByProvider(t *testing.T) {
	cfg := config.Default()
	cfg.ProjectRoot = t.TempDir()
	cfg.Brain.Enabled = false
	cfg.Routing.Default.Provider = "anthropic"
	cfg.Routing.Default.Model = "claude-sonnet-4-20250514"
	cfg.Providers = map[string]config.ProviderConfig{
		"anthropic": {Type: "anthropic", Model: "claude-sonnet-4-20250514"},
		"openai":    {Type: "openai", Model: "gpt-4o"},
	}

	runtime := &stubRuntimeInspector{models: []provider.Model{
		{ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4", Provider: "anthropic", ContextWindow: 200000},
		{ID: "gpt-4o", Name: "GPT-4o", Provider: "openai", ContextWindow: 128000},
	}, authStatuses: map[string]*provider.AuthStatus{
		"anthropic": {Provider: "anthropic", Mode: "oauth", Source: "credential_file", StorePath: "/tmp/.claude/.credentials.json", HasAccessToken: true, HasRefreshToken: true},
		"openai":    {Provider: "openai", Detail: "auth status unavailable"},
	}, health: map[string]*router.ProviderHealth{
		"anthropic": {Healthy: true},
		"openai":    {Healthy: false, LastError: context.DeadlineExceeded},
	}}

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewConfigHandler(srv, cfg, runtime, nil, newTestLogger())
	_, base := startServer(t, srv)

	resp, err := http.Get(base + "/api/config")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var body struct {
		Providers []struct {
			Name   string   `json:"name"`
			Models []string `json:"models"`
		} `json:"providers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	sort.Slice(body.Providers, func(i, j int) bool { return body.Providers[i].Name < body.Providers[j].Name })

	if len(body.Providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(body.Providers))
	}

	// Anthropic should only have claude model.
	if len(body.Providers[0].Models) != 1 || body.Providers[0].Models[0] != "claude-sonnet-4-20250514" {
		t.Fatalf("anthropic models in /api/config: %v", body.Providers[0].Models)
	}

	// OpenAI should only have gpt model.
	if len(body.Providers[1].Models) != 1 || body.Providers[1].Models[0] != "gpt-4o" {
		t.Fatalf("openai models in /api/config: %v", body.Providers[1].Models)
	}
}

func TestConfigAndProviderEndpointsHideDefaultInjectedProvidersWhenConfigSpecifiedOnes(t *testing.T) {
	cfg := config.Default()
	cfg.ProjectRoot = t.TempDir()
	cfg.Brain.Enabled = false
	cfg.ConfiguredProviders = []string{"codex"}
	cfg.Providers = map[string]config.ProviderConfig{
		"anthropic":  {Type: "anthropic", Model: "claude-sonnet-4-20250514"},
		"openrouter": {Type: "openai-compatible", Model: "anthropic/claude-sonnet-4"},
		"codex":      {Type: "codex", Model: "gpt-5.4-mini"},
	}

	runtime := &stubRuntimeInspector{models: []provider.Model{
		{ID: "claude-sonnet-4-20250514", Provider: "anthropic"},
		{ID: "anthropic/claude-sonnet-4", Provider: "openrouter"},
		{ID: "gpt-5.4-mini", Provider: "codex"},
	}, authStatuses: map[string]*provider.AuthStatus{
		"codex": {Provider: "codex", Mode: "oauth", Source: "sirtopham_store", HasAccessToken: true},
	}, health: map[string]*router.ProviderHealth{
		"anthropic":  {Healthy: true},
		"openrouter": {Healthy: true},
		"codex":      {Healthy: true},
	}}

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewConfigHandler(srv, cfg, runtime, nil, newTestLogger())
	_, base := startServer(t, srv)

	for _, path := range []string{"/api/config", "/api/providers", "/api/auth/providers"} {
		resp, err := http.Get(base + path)
		if err != nil {
			t.Fatalf("request %s failed: %v", path, err)
		}
		defer resp.Body.Close()
		var body struct {
			Providers []struct {
				Name string `json:"name"`
			} `json:"providers"`
		}
		var names []string
		if path == "/api/config" {
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("decode %s: %v", path, err)
			}
			for _, provider := range body.Providers {
				names = append(names, provider.Name)
			}
		} else {
			var entries []struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
				t.Fatalf("decode %s: %v", path, err)
			}
			for _, provider := range entries {
				names = append(names, provider.Name)
			}
		}
		if !slices.Equal(names, []string{"codex"}) {
			t.Fatalf("providers for %s = %#v, want [codex]", path, names)
		}
	}
}

func TestConfigEndpointUsesAvailableRuntimeModelsEvenIfAuthStatusesFail(t *testing.T) {
	cfg := config.Default()
	cfg.ProjectRoot = t.TempDir()
	cfg.Brain.Enabled = false
	cfg.Routing.Default.Provider = "anthropic"
	cfg.Routing.Default.Model = "config-default-model"
	cfg.Providers = map[string]config.ProviderConfig{
		"anthropic": {Type: "anthropic", Model: "config-anthropic-model"},
		"openai":    {Type: "openai", Model: "config-openai-model"},
	}

	runtime := &stubRuntimeInspector{
		models: []provider.Model{
			{ID: "runtime-anthropic-model", Provider: "anthropic"},
			{ID: "runtime-openai-model", Provider: "openai"},
		},
		authErr: errors.New("auth status backend unavailable"),
		health: map[string]*router.ProviderHealth{
			"anthropic": {Healthy: true},
			"openai":    {Healthy: false, LastError: context.DeadlineExceeded},
		},
	}

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewConfigHandler(srv, cfg, runtime, nil, newTestLogger())
	_, base := startServer(t, srv)

	resp, err := http.Get(base + "/api/config")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var body struct {
		Providers []struct {
			Name    string   `json:"name"`
			Models  []string `json:"models"`
			Healthy bool     `json:"healthy"`
			Status  string   `json:"status"`
			Auth    any      `json:"auth"`
		} `json:"providers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	sort.Slice(body.Providers, func(i, j int) bool { return body.Providers[i].Name < body.Providers[j].Name })
	if len(body.Providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(body.Providers))
	}
	if got := body.Providers[0].Models; len(got) != 1 || got[0] != "runtime-anthropic-model" {
		t.Fatalf("expected runtime anthropic models, got %v", got)
	}
	if got := body.Providers[1].Models; len(got) != 1 || got[0] != "runtime-openai-model" {
		t.Fatalf("expected runtime openai models, got %v", got)
	}
	if body.Providers[1].Healthy {
		t.Fatalf("expected unhealthy openai runtime health, got %+v", body.Providers[1])
	}
	if body.Providers[1].Status != "unavailable" {
		t.Fatalf("expected unavailable openai status, got %+v", body.Providers[1])
	}
	if body.Providers[0].Auth != nil || body.Providers[1].Auth != nil {
		t.Fatalf("expected auth payloads to be omitted when auth status lookup fails, got %+v", body.Providers)
	}
}

func TestPutConfigRejectsModelNotAvailableOnProvider(t *testing.T) {
	cfg := config.Default()
	cfg.ProjectRoot = t.TempDir()
	cfg.Brain.Enabled = false
	cfg.Providers = map[string]config.ProviderConfig{
		"anthropic": {Type: "anthropic", Model: "claude-sonnet-4-20250514"},
		"openai":    {Type: "openai", Model: "gpt-4o"},
	}

	runtime := &stubRuntimeInspector{
		models: []provider.Model{
			{ID: "claude-sonnet-4-20250514", Provider: "anthropic"},
			{ID: "gpt-4o", Provider: "openai"},
		},
	}

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewConfigHandler(srv, cfg, runtime, nil, newTestLogger())
	_, base := startServer(t, srv)

	body := []byte(`{"default_provider":"anthropic","default_model":"gpt-4o"}`)
	req, err := http.NewRequest(http.MethodPut, base+"/api/config", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("PUT /api/config status = %d, want 400", resp.StatusCode)
	}
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error payload: %v", err)
	}
	if !strings.Contains(payload.Error, "model gpt-4o not available on provider anthropic") {
		t.Fatalf("error = %q, want unavailable-model message", payload.Error)
	}
}
